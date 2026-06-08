package comrad

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// WorkerStatusSnapshot is the stable JSON contract consumed by all desktop clients.
// Additive changes only — field names and types must not change.
type WorkerStatusSnapshot struct {
	Connected       bool             `json:"connected"`
	NodeID          string           `json:"nodeId"`
	NodeName        string           `json:"nodeName"`
	Version         string           `json:"version"`
	Target          string           `json:"target"`
	RuntimeAdapters []string         `json:"runtimeAdapters"`
	Slots           []SlotStatus     `json:"slots"`
	CachedCount     int              `json:"cachedCount"`
	WarmCount       int              `json:"warmCount"`
	WarmProfiles    []string         `json:"warmProfiles"`
	P2P             *WorkerP2PStatus `json:"p2p,omitempty"`
	ManagerURL      string           `json:"managerUrl"`
	LastError       string           `json:"lastError,omitempty"`
	Paused          bool             `json:"paused,omitempty"`
	StartedAt       time.Time        `json:"startedAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
}

// SlotStatus is a minimal slot representation in the status snapshot.
type SlotStatus struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// StatusSnapshot builds a point-in-time snapshot of the worker's state.
func (w *Worker) StatusSnapshot() WorkerStatusSnapshot {
	w.refreshP2PState()

	w.mu.Lock()
	defer w.mu.Unlock()

	slots := make([]SlotStatus, 0, len(w.slots))
	for _, s := range w.slots {
		slots = append(slots, SlotStatus{ID: s.ID, State: s.State})
	}

	warmProfiles := make([]string, 0, len(w.warm))
	for id := range w.warm {
		warmProfiles = append(warmProfiles, id)
	}

	adapters := w.node.RuntimeAdapters
	if adapters == nil {
		adapters = []string{}
	}

	return WorkerStatusSnapshot{
		Connected:       w.connected,
		NodeID:          w.node.ID,
		NodeName:        w.node.Name,
		Version:         w.node.Version,
		Target:          w.node.Target,
		RuntimeAdapters: adapters,
		Slots:           slots,
		CachedCount:     len(w.cache),
		WarmCount:       len(w.warm),
		WarmProfiles:    warmProfiles,
		P2P:             w.node.P2P,
		ManagerURL:      w.cfg.ManagerURL,
		LastError:       w.lastError,
		Paused:          w.paused.Load(),
		StartedAt:       w.startedAt,
		UpdatedAt:       time.Now().UTC(),
	}
}

// setPaused enables or disables the idle-pause mode. When paused, slots that are
// ready are reported as idle to the manager so no new tasks are assigned. Active
// tasks continue to completion. When unpaused, ready slots are re-advertised.
func (w *Worker) setPaused(paused bool) {
	if w.paused.Swap(paused) == paused {
		return // no change
	}
	w.mu.Lock()
	slots := make([]Slot, 0, len(w.slots))
	for _, s := range w.slots {
		slots = append(slots, s)
	}
	w.mu.Unlock()
	for _, slot := range slots {
		if slot.State == SlotStateReady {
			w.sendSlotState(slot) // sendSlotState applies the pause filter
		}
	}
}

// ServeStatus starts a loopback-only HTTP server at addr exposing /status and /healthz.
// addr must be 127.0.0.1:<port> or ::1:<port>; any other host returns an error.
// The server shuts down when ctx is cancelled.
func (w *Worker) ServeStatus(ctx context.Context, addr string) error {
	return w.serveStatus(ctx, addr, nil)
}

// serveStatus is ServeStatus with an optional addrCh that receives the bound address
// (useful in tests to discover the OS-assigned port when ":0" is passed).
func (w *Worker) serveStatus(ctx context.Context, addr string, addrCh chan<- string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("status addr %q: %w", addr, err)
	}
	if host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("status addr %q: only loopback addresses (127.0.0.1, ::1) are allowed", addr)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("status listen %q: %w", addr, err)
	}

	if addrCh != nil {
		addrCh <- ln.Addr().String()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(rw http.ResponseWriter, _ *http.Request) {
		snap := w.StatusSnapshot()
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(snap)
	})
	mux.HandleFunc("/healthz", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/pause", func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.setPaused(true)
		rw.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/resume", func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.setPaused(false)
		rw.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}
