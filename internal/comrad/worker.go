package comrad

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WorkerConfig struct {
	ManagerURL       string
	Token            string
	NodeID           string
	Name             string
	StatePath        string
	CacheDir         string
	LlamaServerPath  string
	RuntimeStartWait time.Duration
	SlotCount        int
	RAMBytes         int64
	VRAMBytes        int64
	UnifiedBytes     int64
	DiskBytes        int64
	EnableSelfUpdate bool
}

type Worker struct {
	cfg             WorkerConfig
	client          *http.Client
	node            Node
	nodeToken       string
	slots           map[string]Slot
	assigns         map[string]AssignmentPayload
	cache           map[string]string
	warm            map[string]WorkloadProfile
	processed       map[string]time.Time
	active          map[string]context.CancelFunc
	runtimes        map[string]*llamaServerProcess
	runtimeRestarts map[string]int
	conn            *websocket.Conn
	send            chan Envelope
	mu              sync.Mutex
}

type workerStateFile struct {
	NodeID      string            `json:"nodeId"`
	NodeToken   string            `json:"nodeToken,omitempty"`
	Cache       map[string]string `json:"cache"`
	WarmProfile []string          `json:"warmProfiles"`
}

func NewWorker(cfg WorkerConfig) (*Worker, error) {
	applyWorkerDefaults(&cfg)
	if err := EnsureDir(filepath.Dir(cfg.StatePath)); err != nil {
		return nil, err
	}
	if err := EnsureDir(cfg.CacheDir); err != nil {
		return nil, err
	}
	state := loadWorkerState(cfg.StatePath)
	cfg.NodeID = workerNodeID(cfg.NodeID, state.NodeID)
	target, adapters := workerRuntimeTarget(cfg)
	node := newWorkerNode(cfg, target, adapters)
	w := &Worker{
		cfg:             cfg,
		client:          &http.Client{Timeout: 0},
		node:            node,
		nodeToken:       state.NodeToken,
		slots:           newWorkerSlots(cfg, target, adapters),
		assigns:         map[string]AssignmentPayload{},
		cache:           state.Cache,
		warm:            map[string]WorkloadProfile{},
		processed:       map[string]time.Time{},
		active:          map[string]context.CancelFunc{},
		runtimes:        map[string]*llamaServerProcess{},
		runtimeRestarts: map[string]int{},
		send:            make(chan Envelope, 256),
	}
	if w.cache == nil {
		w.cache = map[string]string{}
	}
	if err := w.saveState(); err != nil {
		return nil, err
	}
	return w, nil
}

func applyWorkerDefaults(cfg *WorkerConfig) {
	if cfg.ManagerURL == "" {
		cfg.ManagerURL = "http://127.0.0.1:1922"
	}
	if cfg.Token == "" {
		cfg.Token = "dev-worker-token"
	}
	if cfg.StatePath == "" {
		cfg.StatePath = "data/worker-state.json"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = "data/worker-cache"
	}
	if cfg.SlotCount <= 0 {
		cfg.SlotCount = 1
	}
	if cfg.RAMBytes <= 0 {
		cfg.RAMBytes = 8 << 30
	}
	if cfg.UnifiedBytes <= 0 {
		cfg.UnifiedBytes = cfg.RAMBytes
	}
	if cfg.DiskBytes <= 0 {
		cfg.DiskBytes = 20 << 30
	}
	if cfg.RuntimeStartWait <= 0 {
		cfg.RuntimeStartWait = 60 * time.Second
	}
}

func loadWorkerState(path string) workerStateFile {
	state := workerStateFile{Cache: map[string]string{}}
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &state)
	}
	return state
}

func workerNodeID(configID, stateID string) string {
	if configID != "" {
		return configID
	}
	if stateID != "" {
		return stateID
	}
	return NewID("node")
}

func workerRuntimeTarget(cfg WorkerConfig) (string, []string) {
	target := runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		if llamaServerExecutablePath(cfg) != "" {
			return TargetDarwinArm64Metal, []string{"llama.cpp-metal"}
		}
		return TargetDarwinArm64Metal, []string{}
	}
	return target, []string{}
}

func newWorkerNode(cfg WorkerConfig, target string, adapters []string) Node {
	return Node{ID: cfg.NodeID, Name: workerNodeName(cfg), OS: runtime.GOOS, Arch: runtime.GOARCH, Target: target, Mode: "reserved_always", Tags: []string{"local", runtime.GOOS, runtime.GOARCH}, State: NodeStateRegistered, Version: Version, RuntimeAdapters: adapters, Budgets: workerBudget(cfg, cfg.SlotCount), Approved: true}
}

func workerNodeName(cfg WorkerConfig) string {
	if cfg.Name != "" {
		return cfg.Name
	}
	if host, _ := os.Hostname(); host != "" {
		return host
	}
	return cfg.NodeID
}

func workerBudget(cfg WorkerConfig, slotCount int) ResourceBudget {
	return ResourceBudget{RAMBytes: cfg.RAMBytes, VRAMBytes: cfg.VRAMBytes, UnifiedMemoryBytes: cfg.UnifiedBytes, DiskBytes: cfg.DiskBytes, SlotCount: slotCount}
}

func newWorkerSlots(cfg WorkerConfig, target string, adapters []string) map[string]Slot {
	slots := map[string]Slot{}
	for i := 0; i < cfg.SlotCount; i++ {
		id := fmt.Sprintf("%s/metal%d", cfg.NodeID, i)
		slots[id] = Slot{ID: id, NodeID: cfg.NodeID, Target: target, RuntimeAdapter: firstRuntimeAdapter(adapters), Resources: workerBudget(cfg, 1), State: SlotStateIdle, AcceptsNew: false}
	}
	return slots
}

func firstRuntimeAdapter(adapters []string) string {
	if len(adapters) == 0 {
		return ""
	}
	return adapters[0]
}

func (w *Worker) Run(ctx context.Context) error {
	defer w.stopAllRuntimeServers()
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := w.connectAndServe(ctx); err != nil {
			log.Printf("worker disconnected: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func (w *Worker) connectAndServe(ctx context.Context) error {
	wsURL, err := workerWSURL(w.cfg.ManagerURL)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, workerWSHeaders(w.cfg.Token))
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.conn = conn
	w.send = make(chan Envelope, 256)
	w.mu.Unlock()
	defer conn.Close()
	errCh := make(chan error, 2)
	go w.writeLoop(ctx, conn, errCh)
	go w.readLoop(ctx, conn, errCh)
	w.sendHello()
	w.sendFullState()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.enqueue(Envelope{ID: NewID("msg"), Type: MsgHeartbeat, NodeID: w.node.ID})
		case err := <-errCh:
			return err
		}
	}
}

func workerWSURL(managerURL string) (string, error) {
	u, err := url.Parse(managerURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported manager URL scheme %s", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/worker/ws"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func workerWSHeaders(token string) http.Header {
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	return headers
}

func (w *Worker) writeLoop(ctx context.Context, conn *websocket.Conn, errCh chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-w.send:
			_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if err := conn.WriteJSON(msg); err != nil {
				errCh <- err
				return
			}
		}
	}
}

func (w *Worker) readLoop(ctx context.Context, conn *websocket.Conn, errCh chan<- error) {
	for {
		var msg Envelope
		if err := conn.ReadJSON(&msg); err != nil {
			errCh <- err
			return
		}
		if err := w.handleEnvelope(ctx, msg); err != nil {
			log.Printf("manager message error: %v", err)
			w.enqueue(Envelope{ID: msg.ID, Type: MsgError, NodeID: w.node.ID, Payload: MarshalPayload(map[string]string{"error": err.Error()})})
		}
	}
}

func (w *Worker) enqueue(msg Envelope) bool {
	w.mu.Lock()
	send := w.send
	w.mu.Unlock()
	select {
	case send <- msg:
		return true
	default:
		return false
	}
}

func (w *Worker) sendHello() {
	w.mu.Lock()
	node := w.node
	nodeToken := w.nodeToken
	slots := make([]Slot, 0, len(w.slots))
	for _, slot := range w.slots {
		slots = append(slots, slot)
	}
	w.mu.Unlock()
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgHello, NodeID: node.ID, Payload: MarshalPayload(HelloPayload{NodeToken: nodeToken, Node: node, Slots: slots})})
}

func (w *Worker) sendFullState() {
	w.mu.Lock()
	node := w.node
	nodeToken := w.nodeToken
	slots := make([]Slot, 0, len(w.slots))
	for _, slot := range w.slots {
		slots = append(slots, slot)
	}
	cached := make([]string, 0, len(w.cache))
	for id := range w.cache {
		cached = append(cached, id)
	}
	warm := make([]string, 0, len(w.warm))
	for id := range w.warm {
		warm = append(warm, id)
	}
	processed := make([]string, 0, len(w.processed))
	for id := range w.processed {
		processed = append(processed, id)
	}
	w.mu.Unlock()
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgFullState, NodeID: node.ID, Payload: MarshalPayload(FullStatePayload{NodeToken: nodeToken, Node: node, Slots: slots, Cached: cached, WarmProfiles: warm, ProcessedIDs: processed})})
}

func (w *Worker) handleEnvelope(ctx context.Context, msg Envelope) error {
	if msg.ID != "" && w.seen(msg.ID) {
		w.enqueue(Envelope{ID: msg.ID, Type: MsgAck, NodeID: w.node.ID})
		return nil
	}
	handlers := workerInboundHandlers()
	if handler, ok := handlers[msg.Type]; ok {
		return handler(w, ctx, msg)
	}
	return fmt.Errorf("unsupported message %s", msg.Type)
}

type workerInboundHandler func(*Worker, context.Context, Envelope) error

func workerInboundHandlers() map[string]workerInboundHandler {
	return map[string]workerInboundHandler{MsgAck: workerAck, MsgAssignProfile: workerAssignProfile, MsgExecuteTask: workerExecuteTask, MsgCancelTask: workerCancelTask, MsgEvictArtifact: workerEvictArtifact, MsgUpdateWorker: workerUpdate}
}

func (w *Worker) seen(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.processed[id]; ok {
		return true
	}
	w.processed[id] = time.Now().UTC()
	if len(w.processed) > 2048 {
		cutoff := time.Now().Add(-30 * time.Minute)
		for k, t := range w.processed {
			if t.Before(cutoff) {
				delete(w.processed, k)
			}
		}
	}
	return false
}
