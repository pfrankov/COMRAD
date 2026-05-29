package comrad

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (m *Manager) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	db := m.store.Snapshot()
	models := []map[string]any{}
	for _, profile := range SortedProfiles(db) {
		if profile.Kind != "llm.chat" || minimumSufficientContext(profile, 1) == 0 {
			continue
		}
		models = append(models, map[string]any{
			"id":           profile.ID,
			"object":       "model",
			"owned_by":     "comrad",
			"alias":        profile.Alias,
			"logicalModel": ProfileLogicalModel(profile),
			"context":      minimumSufficientContext(profile, 1),
			"variants":     ProfileRuntimeVariants(profile),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

func (m *Manager) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	req, opts, ok := parseChatRequest(w, r)
	if !ok {
		return
	}
	user := requestUser(r)
	profile, ok := m.resolveChatProfile(w, req, opts.minContext)
	if !ok {
		return
	}
	release := m.acquireQueue(w)
	if release == nil {
		return
	}
	defer release()
	task, ok := m.createChatTask(w, req, opts.minContext, profile, user)
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if req.Stream {
		m.streamChat(ctx, w, task, profile, req.Messages, opts.maxTokens, opts.temperature)
		return
	}
	content, status, err := m.collectChat(ctx, task, profile, req.Messages, opts.maxTokens, opts.temperature)
	if err != nil {
		writeError(w, status, errorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chatCompletion(task.ID, profile.ID, content))
}

type chatOptions struct {
	minContext  int
	maxTokens   int
	temperature float64
}

func parseChatRequest(w http.ResponseWriter, r *http.Request) (ChatCompletionRequest, chatOptions, bool) {
	var req ChatCompletionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return req, chatOptions{}, false
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "model is required")
		return req, chatOptions{}, false
	}
	return req, defaultChatOptions(req), true
}

func defaultChatOptions(req ChatCompletionRequest) chatOptions {
	opts := chatOptions{minContext: req.MinContext, maxTokens: req.MaxTokens, temperature: req.Temperature}
	if req.Comrad.MinContextTokens > opts.minContext {
		opts.minContext = req.Comrad.MinContextTokens
	}
	if opts.minContext == 0 {
		opts.minContext = 1
	}
	if opts.maxTokens <= 0 {
		opts.maxTokens = 128
	}
	if opts.temperature == 0 {
		opts.temperature = 0.2
	}
	return opts
}

func (m *Manager) acquireQueue(w http.ResponseWriter) func() {
	select {
	case m.queue <- struct{}{}:
		return func() { <-m.queue }
	default:
		writeError(w, http.StatusServiceUnavailable, FailureNoCapacity, "global queue is full")
		return nil
	}
}

func (m *Manager) resolveChatProfile(w http.ResponseWriter, req ChatCompletionRequest, minContext int) (WorkloadProfile, bool) {
	profile, err := ResolveLLMProfile(m.store.Snapshot(), req.Model, minContext)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "no exact compatible profile satisfies the request")
		return WorkloadProfile{}, false
	}
	return profile, true
}

func (m *Manager) createChatTask(w http.ResponseWriter, req ChatCompletionRequest, minContext int, profile WorkloadProfile, user User) (Task, bool) {
	task := newChatTask(req, profile, minContext, user)
	err := m.store.Update(func(db *Database) error { return storeNewTask(db, task, m.cfg.EnforceBalance) })
	if errors.Is(err, errInsufficientBalance) {
		writeError(w, http.StatusPaymentRequired, err.Error(), "compute balance is insufficient for the selected profile")
		return Task{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return Task{}, false
	}
	return task, true
}

func newChatTask(req ChatCompletionRequest, profile WorkloadProfile, minContext int, user User) Task {
	now := time.Now().UTC()
	return Task{ID: NewID("task"), UserID: user.ID, Kind: "llm.chat", Model: req.Model, ProfileID: profile.ID, ProfileVersion: profileVersion(profile), LogicalModel: ProfileLogicalModel(profile), ComputeCost: profile.ComputeCost, MinContextTokens: minContext, Status: TaskStatusQueued, Stream: req.Stream, CreatedAt: now, UpdatedAt: now}
}

func storeNewTask(db *Database, task Task, enforceBalance bool) error {
	if err := ensureSufficientAvailableBalance(*db, task.UserID, task.ComputeCost, enforceBalance); err != nil {
		return err
	}
	db.Tasks[task.ID] = task
	db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "task.created", Actor: "client", Subject: task.ID, CreatedAt: task.CreatedAt})
	return nil
}

func chatCompletion(taskID, profileID, content string) ChatCompletionChunk {
	return ChatCompletionChunk{ID: taskID, Object: "chat.completion", Model: profileID, Choices: []ChatCompletionChoice{{Index: 0, Message: &ChatMessage{Role: "assistant", Content: content}, FinishReason: "stop"}}}
}

func (m *Manager) handleJobs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	if path == "" {
		writeError(w, http.StatusNotFound, "not_found", "job id required")
		return
	}
	if strings.HasSuffix(path, "/cancel") {
		taskID := strings.TrimSuffix(path, "/cancel")
		taskID = strings.TrimSuffix(taskID, "/")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if _, ok := m.clientTask(r, taskID); !ok {
			writeError(w, http.StatusNotFound, "not_found", "job not found")
			return
		}
		m.cancelTask(taskID, FailureCancelledByClient)
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	task, ok := m.clientTask(r, path)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (m *Manager) clientTask(r *http.Request, taskID string) (Task, bool) {
	db := m.store.Snapshot()
	task, ok := db.Tasks[taskID]
	if !ok {
		return Task{}, false
	}
	user := requestUser(r)
	if task.UserID != "" && task.UserID != user.ID {
		return Task{}, false
	}
	return task, true
}
