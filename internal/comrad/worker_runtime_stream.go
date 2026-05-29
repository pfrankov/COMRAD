package comrad

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

func (w *Worker) streamRuntimeTokens(ctx context.Context, payload ExecuteTaskPayload, emit func(string) error) (runtimeStreamStats, error) {
	proc, err := w.runtimeServerForPayload(payload)
	if err != nil {
		return runtimeStreamStats{}, err
	}
	body, err := json.Marshal(llamaChatRequest{
		Model:       ProfileLogicalModel(payload.Profile),
		Messages:    payload.Messages,
		MaxTokens:   payload.MaxTokens,
		Temperature: payload.Temperature,
		Stream:      true,
	})
	if err != nil {
		return runtimeStreamStats{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, proc.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return runtimeStreamStats{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := w.client.Do(req)
	if err != nil {
		return runtimeStreamStats{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return runtimeStreamStats{}, fmt.Errorf("llama-server chat failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return readLlamaServerStream(resp.Body, emit)
}

func (w *Worker) runtimeServerForPayload(payload ExecuteTaskPayload) (*llamaServerProcess, error) {
	w.mu.Lock()
	proc := w.runtimes[payload.SlotID]
	w.mu.Unlock()
	if proc == nil || proc.profileKey != assignmentKey(payload.Profile) || !proc.alive() {
		return nil, errors.New("llama-server is not ready for this slot")
	}
	return proc, nil
}

type llamaChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type llamaStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage   *llamaUsage   `json:"usage"`
	Timings *llamaTimings `json:"timings"`
}

type llamaUsage struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
}

type llamaTimings struct {
	PromptN            *int     `json:"prompt_n"`
	PredictedN         *int     `json:"predicted_n"`
	PredictedMS        *float64 `json:"predicted_ms"`
	PredictedPerSecond *float64 `json:"predicted_per_second"`
}

func readLlamaServerStream(r io.Reader, emit func(string) error) (runtimeStreamStats, error) {
	stats := runtimeStreamStats{}
	done := false
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			done = true
			break
		}
		if err := emitLlamaStreamPayload(payload, emit, &stats); err != nil {
			return stats, err
		}
	}
	if err := scanner.Err(); err != nil {
		return stats, err
	}
	if !done {
		return stats, errors.New("llama-server stream ended before done")
	}
	return stats, nil
}

func emitLlamaStreamPayload(payload string, emit func(string) error, stats *runtimeStreamStats) error {
	var chunk llamaStreamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return err
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content == "" {
			continue
		}
		if stats.FirstTokenAt == nil {
			now := time.Now().UTC()
			stats.FirstTokenAt = &now
		}
		stats.CompletionTokens += countCompletionTokens(choice.Delta.Content)
		if err := emit(choice.Delta.Content); err != nil {
			return err
		}
	}
	applyLlamaStreamMetrics(&chunk, stats)
	return nil
}

func applyLlamaStreamMetrics(chunk *llamaStreamChunk, stats *runtimeStreamStats) {
	if chunk.Usage != nil {
		if chunk.Usage.PromptTokens != nil {
			stats.PromptTokens = *chunk.Usage.PromptTokens
			stats.HasPromptTokens = true
		}
		if chunk.Usage.CompletionTokens != nil {
			stats.CompletionTokens = *chunk.Usage.CompletionTokens
		}
	}
	if chunk.Timings == nil {
		return
	}
	if chunk.Timings.PromptN != nil {
		stats.PromptTokens = *chunk.Timings.PromptN
		stats.HasPromptTokens = true
	}
	if chunk.Timings.PredictedN != nil {
		stats.CompletionTokens = *chunk.Timings.PredictedN
	}
	if chunk.Timings.PredictedMS != nil {
		stats.GenerationMS = int64(math.Round(*chunk.Timings.PredictedMS))
		stats.HasGeneration = true
	}
	if chunk.Timings.PredictedPerSecond != nil {
		stats.TokensPerSecond = *chunk.Timings.PredictedPerSecond
		stats.HasTokensPerSec = true
	}
	if !stats.HasTokensPerSec && stats.HasGeneration && stats.GenerationMS > 0 {
		stats.TokensPerSecond = float64(stats.CompletionTokens) / (float64(stats.GenerationMS) / 1000.0)
		stats.HasTokensPerSec = true
	}
}

func countCompletionTokens(s string) int {
	count := len(strings.Fields(s))
	if count == 0 && s != "" {
		return 1
	}
	return count
}

func promptTokens(messages []ChatMessage) int {
	n := 0
	for _, msg := range messages {
		n += len(strings.Fields(msg.Content))
	}
	return n
}
