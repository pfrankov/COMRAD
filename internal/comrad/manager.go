package comrad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type ManagerConfig struct {
	Addr                         string
	DBPath                       string
	DatabaseURL                  string
	StorageMode                  string
	ArtifactDir                  string
	AdminToken                   string
	ClientAPIKey                 string
	WorkerToken                  string
	EnforceBalance               bool
	QueueLimit                   int
	StreamWait                   time.Duration
	AutoBalanceScaleDownCooldown time.Duration
	QuarantineThreshold          int
	QuarantineDuration           time.Duration
	WorkerHeartbeatTimeout       time.Duration
	WorkerFlapThreshold          int
	WorkerFlapWindow             time.Duration
	WorkerFlapCooldown           time.Duration
	AutoApprove                  bool
	ExternalURL                  string
	AllowDevTokens               bool
}

type Manager struct {
	cfg      ManagerConfig
	store    *Store
	upgrader websocket.Upgrader
	queue    chan struct{}

	mu                    sync.Mutex
	sessions              map[string]*workerSession
	streams               map[string]*activeAttempt
	adminStateSubscribers map[string]chan StateResponse
	adminStateWSTickets   map[string]time.Time
	runtimeMetrics        RuntimeMetrics
}

type activeAttempt struct {
	taskID      string
	attemptID   string
	firstOutput bool
	events      chan streamEvent
	createdAt   time.Time
	cancelOnce  sync.Once
	cancelFn    func()
}

type streamEvent struct {
	kind   string
	token  TokenPayload
	failed AttemptFailedPayload
	report ComputeReport
	err    error
}

type workerSession struct {
	id         string
	nodeID     string
	baseURL    string
	remoteHost string
	conn       *websocket.Conn
	manager    *Manager
	send       chan Envelope
	done       chan struct{}
	once       sync.Once
}

func NewManager(cfg ManagerConfig) (*Manager, error) {
	applyManagerDefaults(&cfg)
	if err := validateManagerSecrets(cfg); err != nil {
		return nil, err
	}
	if err := EnsureDir(cfg.ArtifactDir); err != nil {
		return nil, err
	}
	store, err := OpenConfiguredStore(StoreConfig{Mode: cfg.StorageMode, DatabaseURL: cfg.DatabaseURL, SQLitePath: cfg.DBPath})
	if err != nil {
		return nil, err
	}
	m := &Manager{
		cfg:                   cfg,
		store:                 store,
		queue:                 make(chan struct{}, cfg.QueueLimit),
		sessions:              map[string]*workerSession{},
		streams:               map[string]*activeAttempt{},
		adminStateSubscribers: map[string]chan StateResponse{},
		adminStateWSTickets:   map[string]time.Time{},
		upgrader:              websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
	m.store.SetAfterUpdate(m.publishAdminState)
	if err := m.ensureConfiguredClientKey(); err != nil {
		return nil, err
	}
	if err := m.migrateArtifactTorrentMetadata(); err != nil {
		return nil, err
	}
	return m, nil
}

func applyManagerDefaults(cfg *ManagerConfig) {
	applyManagerPathDefaults(cfg)
	applyManagerRuntimeDefaults(cfg)
	applyWorkerHealthDefaults(cfg)
	if cfg.AllowDevTokens {
		applyDevTokenDefaults(cfg)
	}
}

func applyManagerPathDefaults(cfg *ManagerConfig) {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:1922"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "data/comrad.sqlite"
	}
	cfg.StorageMode = normalizeStorageMode(cfg.StorageMode)
	if cfg.ArtifactDir == "" {
		cfg.ArtifactDir = "data/artifacts"
	}
}

func applyManagerRuntimeDefaults(cfg *ManagerConfig) {
	if cfg.QueueLimit <= 0 {
		cfg.QueueLimit = 32
	}
	if cfg.StreamWait <= 0 {
		cfg.StreamWait = 15 * time.Second
	}
	if cfg.AutoBalanceScaleDownCooldown <= 0 {
		cfg.AutoBalanceScaleDownCooldown = defaultAutoBalanceScaleDownCooldown
	}
	if cfg.QuarantineThreshold <= 0 {
		cfg.QuarantineThreshold = 3
	}
	if cfg.QuarantineDuration <= 0 {
		cfg.QuarantineDuration = 5 * time.Minute
	}
}

func applyWorkerHealthDefaults(cfg *ManagerConfig) {
	if cfg.WorkerHeartbeatTimeout <= 0 {
		cfg.WorkerHeartbeatTimeout = 30 * time.Second
	}
	if cfg.WorkerFlapThreshold <= 0 {
		cfg.WorkerFlapThreshold = 4
	}
	if cfg.WorkerFlapWindow <= 0 {
		cfg.WorkerFlapWindow = 5 * time.Minute
	}
	if cfg.WorkerFlapCooldown <= 0 {
		cfg.WorkerFlapCooldown = 5 * time.Minute
	}
}

const (
	devAdminToken  = "dev-admin-token"
	devClientToken = "dev-client-key"
	devWorkerToken = "dev-worker-token"
)

func applyDevTokenDefaults(cfg *ManagerConfig) {
	if cfg.AdminToken == "" {
		cfg.AdminToken = devAdminToken
	}
	if cfg.ClientAPIKey == "" {
		cfg.ClientAPIKey = devClientToken
	}
	if cfg.WorkerToken == "" {
		cfg.WorkerToken = devWorkerToken
	}
}

func validateManagerSecrets(cfg ManagerConfig) error {
	if cfg.AdminToken == "" {
		return fmt.Errorf("COMRAD_ADMIN_TOKEN is required")
	}
	if cfg.ClientAPIKey == "" {
		return fmt.Errorf("COMRAD_CLIENT_API_KEY is required")
	}
	if cfg.WorkerToken == "" {
		return fmt.Errorf("COMRAD_WORKER_TOKEN is required")
	}
	if !managerUsesDevTokens(cfg) {
		return nil
	}
	if !cfg.AllowDevTokens {
		return fmt.Errorf("dev tokens require COMRAD_ALLOW_DEV_DEFAULTS=true")
	}
	if !managerAddrLoopback(cfg.Addr) {
		return fmt.Errorf("dev tokens require a loopback Manager address")
	}
	return nil
}

func managerUsesDevTokens(cfg ManagerConfig) bool {
	return cfg.AdminToken == devAdminToken || cfg.ClientAPIKey == devClientToken || cfg.WorkerToken == devWorkerToken
}

func managerAddrLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (m *Manager) Store() *Store {
	return m.store
}

func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", m.handleHealth)
	mux.HandleFunc("/ready", m.handleReady)
	mux.HandleFunc("/metrics", m.handleMetrics)
	mux.HandleFunc("/", m.handleDashboard)
	mux.HandleFunc("/api/admin/state", m.adminOnly(m.handleAdminState))
	mux.HandleFunc("/api/admin/state/ws-ticket", m.adminBearerOnly(m.handleAdminStateWSTicket))
	mux.HandleFunc("/api/admin/state/ws", m.handleAdminStateWS)
	mux.HandleFunc("/api/admin/nodes/", m.adminOnly(m.handleAdminNodeArtifactByPath))
	mux.HandleFunc("/api/admin/nodes", m.adminOnly(m.handleAdminNodes))
	mux.HandleFunc("/api/admin/slots", m.adminOnly(m.handleAdminSlots))
	mux.HandleFunc("/api/admin/artifacts/upload", m.adminOnly(m.handleAdminArtifactUpload))
	mux.HandleFunc("/api/admin/artifacts/", m.adminOnly(m.handleAdminArtifactByID))
	mux.HandleFunc("/api/admin/artifacts", m.adminOnly(m.handleAdminArtifacts))
	mux.HandleFunc("/api/admin/profiles", m.adminOnly(m.handleAdminProfiles))
	mux.HandleFunc("/api/admin/profiles/compute-cost", m.adminOnly(m.handleAdminProfileComputeCost))
	mux.HandleFunc("/api/admin/policies", m.adminOnly(m.handleAdminPolicies))
	mux.HandleFunc("/api/admin/users", m.adminOnly(m.handleAdminUsers))
	mux.HandleFunc("/api/admin/users/adjust-balance", m.adminOnly(m.handleAdminBalanceAdjustment))
	mux.HandleFunc("/api/admin/api-keys/lookup", m.adminOnly(m.handleAdminAPIKeyLookup))
	mux.HandleFunc("/api/admin/api-keys", m.adminOnly(m.handleAdminAPIKeys))
	mux.HandleFunc("/api/admin/api-keys/revoke", m.adminOnly(m.handleAdminAPIKeyRevoke))
	mux.HandleFunc("/api/admin/placement/explain", m.adminOnly(m.handleAdminPlacementExplain))
	mux.HandleFunc("/api/admin/placement", m.adminOnly(m.handleAdminPlacement))
	mux.HandleFunc("/api/admin/placement/apply", m.adminOnly(m.handleApplyPlacement))
	mux.HandleFunc("/api/admin/tasks", m.adminOnly(m.handleAdminTasks))
	mux.HandleFunc("/api/admin/attempts", m.adminOnly(m.handleAdminAttempts))
	mux.HandleFunc("/api/admin/reports", m.adminOnly(m.handleAdminReports))
	mux.HandleFunc("/api/admin/quarantine/unban", m.adminOnly(m.handleAdminUnban))
	mux.HandleFunc("/api/admin/updates", m.adminOnly(m.handleAdminUpdates))
	mux.HandleFunc("/api/admin/updates/workers/apply", m.adminOnly(m.handleApplyWorkerUpdate))
	mux.HandleFunc("/api/admin/metrics", m.adminOnly(m.handleAdminMetrics))
	mux.HandleFunc("/api/admin/worker-join", m.adminOnly(m.handleAdminWorkerJoin))
	mux.HandleFunc("/api/admin/config.yaml", m.adminOnly(m.handleAdminConfigYAML))
	mux.HandleFunc("/api/admin/settings", m.adminOnly(m.handleAdminSettings))
	mux.HandleFunc("/api/admin/openapi.json", m.adminOnly(m.handleOpenAPIJSON))
	mux.HandleFunc("/api/admin/docs", m.adminTicketOrBearer(m.handleOpenAPIDocs))
	mux.HandleFunc("/api/worker/ws", m.handleWorkerWS)
	mux.HandleFunc("/api/worker/artifacts/", m.handleWorkerArtifact)
	mux.HandleFunc("/v1/models", m.clientOnly(m.handleModels))
	mux.HandleFunc("/v1/chat/completions", m.clientOnly(m.handleChatCompletions))
	mux.HandleFunc("/v1/jobs/", m.clientOnly(m.handleJobs))
	return mux
}

func (m *Manager) Serve(ctx context.Context) error {
	srv := &http.Server{Addr: m.cfg.Addr, Handler: m.Handler()}
	errCh := make(chan error, 1)
	go m.runWorkerHeartbeatMonitor(ctx.Done())
	go func() {
		log.Printf("comrad manager listening on %s", m.cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (m *Manager) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, "OK\n")
}

func (m *Manager) handleReady(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "store is not initialized")
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, "OK\n")
}
