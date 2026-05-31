package comrad

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const adminStateWSTicketTTL = 30 * time.Second

func (m *Manager) handleAdminStateWSTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, m.issueAdminStateWSTicket(time.Now().UTC()))
}

func (m *Manager) handleAdminStateWS(w http.ResponseWriter, r *http.Request) {
	if !m.adminStateWSAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "admin websocket ticket required")
		return
	}
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	id, states := m.addAdminStateSubscriber()
	defer m.removeAdminStateSubscriber(id)
	done := make(chan struct{})
	go watchWSClose(conn, done)
	if !writeAdminState(conn, m.stateResponse()) {
		m.recordAdminStateWSWriteFailure()
		return
	}
	for {
		select {
		case <-done:
			return
		case state := <-states:
			if !writeAdminState(conn, state) {
				m.recordAdminStateWSWriteFailure()
				return
			}
		}
	}
}

func (m *Manager) issueAdminStateWSTicket(now time.Time) AdminStateWSTicketResponse {
	ticket := NewID("wst")
	expiresAt := now.Add(adminStateWSTicketTTL)
	m.mu.Lock()
	m.purgeExpiredAdminStateWSTicketsLocked(now)
	m.adminStateWSTickets[apiTokenHash(ticket)] = expiresAt
	m.mu.Unlock()
	return AdminStateWSTicketResponse{Ticket: ticket, ExpiresAt: expiresAt}
}

func (m *Manager) adminStateWSAuthorized(r *http.Request) bool {
	if bearerMatches(r, m.cfg.AdminToken) {
		return true
	}
	return m.consumeAdminStateWSTicket(r.URL.Query().Get("ticket"), time.Now().UTC())
}

func (m *Manager) consumeAdminStateWSTicket(ticket string, now time.Time) bool {
	if ticket == "" {
		return false
	}
	hash := apiTokenHash(ticket)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeExpiredAdminStateWSTicketsLocked(now)
	expiresAt, ok := m.adminStateWSTickets[hash]
	if !ok {
		return false
	}
	delete(m.adminStateWSTickets, hash)
	return expiresAt.After(now)
}

func (m *Manager) purgeExpiredAdminStateWSTicketsLocked(now time.Time) {
	for hash, expiresAt := range m.adminStateWSTickets {
		if !expiresAt.After(now) {
			delete(m.adminStateWSTickets, hash)
		}
	}
}

func watchWSClose(conn websocketConn, done chan<- struct{}) {
	defer close(done)
	for {
		if _, _, err := conn.NextReader(); err != nil {
			return
		}
	}
}

func writeAdminState(conn websocketConn, state StateResponse) bool {
	_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return conn.WriteJSON(state) == nil
}

type websocketConn interface {
	NextReader() (int, io.Reader, error)
	SetWriteDeadline(time.Time) error
	WriteJSON(any) error
}

func (m *Manager) addAdminStateSubscriber() (string, chan StateResponse) {
	id := NewID("admstate")
	ch := make(chan StateResponse, 4)
	m.mu.Lock()
	m.adminStateSubscribers[id] = ch
	m.runtimeMetrics.AdminStateWSConnectsTotal++
	m.mu.Unlock()
	return id, ch
}

func (m *Manager) removeAdminStateSubscriber(id string) {
	m.mu.Lock()
	delete(m.adminStateSubscribers, id)
	m.mu.Unlock()
}

func (m *Manager) publishAdminState() {
	subscribers := m.adminStateSubscriberChannels()
	if len(subscribers) == 0 {
		return
	}
	state := m.stateResponse()
	size := adminStateSnapshotSize(state)
	dropped := 0
	for _, ch := range subscribers {
		if sendLatestAdminState(ch, state) {
			dropped++
		}
	}
	m.recordAdminStateBroadcast(len(subscribers), size, dropped)
}

func (m *Manager) adminStateSubscriberChannels() []chan StateResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]chan StateResponse, 0, len(m.adminStateSubscribers))
	for _, ch := range m.adminStateSubscribers {
		out = append(out, ch)
	}
	return out
}

func sendLatestAdminState(ch chan StateResponse, state StateResponse) bool {
	select {
	case ch <- state:
		return false
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- state:
		return true
	default:
		return true
	}
}

func adminStateSnapshotSize(state StateResponse) int {
	b, err := json.Marshal(state)
	if err != nil {
		return 0
	}
	return len(b)
}

func (m *Manager) recordAdminStateBroadcast(subscribers, snapshotBytes, dropped int) {
	m.mu.Lock()
	m.runtimeMetrics.AdminStateWSBroadcastsTotal++
	m.runtimeMetrics.AdminStateWSLastBroadcastSubscribers = subscribers
	m.runtimeMetrics.AdminStateWSLastSnapshotBytes = snapshotBytes
	m.runtimeMetrics.AdminStateWSDroppedUpdatesTotal += int64(dropped)
	m.mu.Unlock()
}

func (m *Manager) recordAdminStateWSWriteFailure() {
	m.mu.Lock()
	m.runtimeMetrics.AdminStateWSWriteFailuresTotal++
	m.mu.Unlock()
}

func (m *Manager) handleWorkerWS(w http.ResponseWriter, r *http.Request) {
	if !m.workerAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "worker enrollment token required")
		return
	}
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s := &workerSession{
		id:      NewID("ses"),
		baseURL: m.baseURL(r),
		conn:    conn,
		manager: m,
		send:    make(chan Envelope, 256),
		done:    make(chan struct{}),
	}
	go s.writeLoop()
	go s.readLoop()
}

func (m *Manager) baseURL(r *http.Request) string {
	if m.cfg.ExternalURL != "" {
		return strings.TrimRight(m.cfg.ExternalURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *workerSession) enqueue(msg Envelope) bool {
	select {
	case <-s.done:
		return false
	case s.send <- msg:
		return true
	default:
		return false
	}
}

func (s *workerSession) close() {
	s.once.Do(func() {
		close(s.done)
		if s.conn != nil {
			_ = s.conn.Close()
		}
		if s.nodeID != "" {
			s.manager.removeSession(s)
			s.manager.publishWorkerDisconnectFailures(s.failRunningAttempts())
		}
	})
}

func (m *Manager) removeSession(s *workerSession) {
	m.mu.Lock()
	if m.sessions[s.nodeID] == s {
		delete(m.sessions, s.nodeID)
	}
	m.mu.Unlock()
}

func (s *workerSession) failRunningAttempts() []AttemptFailedPayload {
	var failed []AttemptFailedPayload
	_ = s.manager.store.Update(func(db *Database) error {
		node := db.Nodes[s.nodeID]
		if node.ConnectedSession == s.id || node.ConnectedSession == "" {
			markDisconnectedNode(db, node)
		}
		now := time.Now().UTC()
		for id, attempt := range db.Attempts {
			if attempt.NodeID != s.nodeID || attempt.Status != TaskStatusRunning {
				continue
			}
			failed = append(failed, failDisconnectedAttempt(db, id, attempt, now, s.manager.cfg))
		}
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "worker.disconnected", Actor: "worker", Subject: s.nodeID, CreatedAt: time.Now().UTC()})
		return nil
	})
	return failed
}

func markDisconnectedNode(db *Database, node Node) {
	node.State = NodeStateOffline
	node.ConnectedSession = ""
	db.Nodes[node.ID] = node
	for id, slot := range db.Slots {
		if slot.NodeID == node.ID && slot.State != SlotStateServing {
			slot.State = SlotStateUnavailable
			slot.AcceptsNew = false
			slot.MismatchReason = FailureWorkerDisconnected
			db.Slots[id] = slot
		}
	}
}

func failDisconnectedAttempt(db *Database, id string, attempt Attempt, now time.Time, cfg ManagerConfig) AttemptFailedPayload {
	attempt.Status = TaskStatusFailed
	attempt.Phase = FailureWorkerDisconnected
	attempt.FailureReason = FailureWorkerDisconnected
	attempt.CanRetry = !attempt.FirstOutputSent
	attempt.CompletedAt = &now
	db.Attempts[id] = attempt
	markDisconnectedSlot(db, attempt, now, cfg)
	markDisconnectedTask(db, attempt, now)
	return disconnectedPayload(attempt)
}

func markDisconnectedSlot(db *Database, attempt Attempt, now time.Time, cfg ManagerConfig) {
	slot := db.Slots[attempt.SlotID]
	slot.ActiveTaskID = ""
	slot.AcceptsNew = false
	slot.State = SlotStateUnavailable
	slot.MismatchReason = FailureWorkerDisconnected
	recordSlotFailure(db, &slot, FailureWorkerDisconnected, now, cfg.QuarantineThreshold, cfg.QuarantineDuration)
	db.Slots[slot.ID] = slot
}

func markDisconnectedTask(db *Database, attempt Attempt, now time.Time) {
	task := db.Tasks[attempt.TaskID]
	task.FailureReason = FailureWorkerDisconnected
	task.UpdatedAt = now
	task.FailedSlots = appendUnique(task.FailedSlots, attempt.SlotID)
	if !attempt.FirstOutputSent {
		task.Status = TaskStatusQueued
		task.RuntimeVariantID = ""
		task.CompletedAt = nil
	} else {
		task.Status = TaskStatusFailed
		task.CompletedAt = &now
	}
	db.Tasks[task.ID] = task
}

func disconnectedPayload(attempt Attempt) AttemptFailedPayload {
	return AttemptFailedPayload{TaskID: attempt.TaskID, AttemptID: attempt.ID, Phase: FailureWorkerDisconnected, FailureReason: FailureWorkerDisconnected, CanRetry: !attempt.FirstOutputSent, FirstOutputSent: attempt.FirstOutputSent}
}

func (m *Manager) publishWorkerDisconnectFailures(failed []AttemptFailedPayload) {
	for _, payload := range failed {
		if active := m.streamForAttempt(payload.AttemptID); active != nil {
			active.events <- streamEvent{kind: "failed", failed: payload}
		}
	}
}

func (s *workerSession) writeLoop() {
	defer s.close()
	for {
		select {
		case <-s.done:
			return
		case msg := <-s.send:
			_ = s.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if err := s.conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}
}

func (s *workerSession) readLoop() {
	defer s.close()
	for {
		if s.manager.cfg.WorkerHeartbeatTimeout > 0 {
			_ = s.conn.SetReadDeadline(time.Now().Add(s.manager.cfg.WorkerHeartbeatTimeout + 5*time.Second))
		}
		var msg Envelope
		if err := s.conn.ReadJSON(&msg); err != nil {
			return
		}
		if err := s.manager.handleWorkerEnvelope(s, msg); err != nil {
			log.Printf("worker message error: %v", err)
			s.enqueue(Envelope{ID: msg.ID, Type: MsgError, Payload: MarshalPayload(map[string]string{"error": err.Error()})})
		}
	}
}
