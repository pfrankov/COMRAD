package comrad

import (
	"net/http"
	"slices"

	"gopkg.in/yaml.v3"
)

type RuntimeConfigYAML struct {
	Version       string                  `yaml:"version"`
	Manager       ManagerConfigYAML       `yaml:"manager"`
	Storage       StorageConfigYAML       `yaml:"storage"`
	Auth          AuthConfigYAML          `yaml:"auth"`
	Scheduler     SchedulerConfigYAML     `yaml:"scheduler"`
	Workers       WorkersConfigYAML       `yaml:"workers"`
	Observability ObservabilityConfigYAML `yaml:"observability"`
}

type ManagerConfigYAML struct {
	Listen    string `yaml:"listen"`
	PublicURL string `yaml:"publicUrl"`
}

type StorageConfigYAML struct {
	Mode        string `yaml:"mode"`
	Backend     string `yaml:"backend"`
	ArtifactDir string `yaml:"artifactDir"`
	SQLitePath  string `yaml:"sqlitePath"`
	DatabaseURL string `yaml:"databaseUrl"`
}

type AuthConfigYAML struct {
	AdminToken            string `yaml:"adminToken"`
	ClientBootstrapKey    string `yaml:"clientBootstrapKey"`
	WorkerEnrollmentToken string `yaml:"workerEnrollmentToken"`
	EnforceBalance        bool   `yaml:"enforceBalance"`
	AllowDevDefaults      bool   `yaml:"allowDevDefaults"`
}

type SchedulerConfigYAML struct {
	QueueLimit                          int                  `yaml:"queueLimit"`
	StreamWaitSeconds                   int64                `yaml:"streamWaitSeconds"`
	AutoBalanceScaleDownCooldownSeconds int64                `yaml:"autoBalanceScaleDownCooldownSeconds"`
	WorkerHeartbeatSeconds              int64                `yaml:"workerHeartbeatSeconds"`
	WorkerFlap                          WorkerFlapConfigYAML `yaml:"workerFlap"`
	Quarantine                          QuarantineConfigYAML `yaml:"quarantine"`
}

type WorkerFlapConfigYAML struct {
	Threshold       int   `yaml:"threshold"`
	WindowSeconds   int64 `yaml:"windowSeconds"`
	CooldownSeconds int64 `yaml:"cooldownSeconds"`
}

type QuarantineConfigYAML struct {
	Threshold int   `yaml:"threshold"`
	Seconds   int64 `yaml:"seconds"`
}

type WorkersConfigYAML struct {
	Connection  string              `yaml:"connection"`
	AutoApprove bool                `yaml:"autoApprove"`
	P2P         WorkerP2PConfigYAML `yaml:"p2p"`
}

type WorkerP2PConfigYAML struct {
	Enabled                         bool    `yaml:"enabled"`
	Mode                            string  `yaml:"mode,omitempty"`
	Discovery                       string  `yaml:"discovery,omitempty"`
	DefaultPort                     int     `yaml:"defaultPort,omitempty"`
	DefaultMaxUploads               int     `yaml:"defaultMaxUploads,omitempty"`
	DefaultDownloadTimeoutSeconds   int64   `yaml:"defaultDownloadTimeoutSeconds,omitempty"`
	AvailableWorkers                int     `yaml:"availableWorkers,omitempty"`
	ReportingWorkers                int     `yaml:"reportingWorkers,omitempty"`
	EffectivePorts                  []int   `yaml:"effectivePorts,omitempty"`
	EffectiveMaxUploads             []int   `yaml:"effectiveMaxUploads,omitempty"`
	EffectiveDownloadTimeoutSeconds []int64 `yaml:"effectiveDownloadTimeoutSeconds,omitempty"`
}

type ObservabilityConfigYAML struct {
	DashboardStateStream string `yaml:"dashboardStateStream"`
}

func (m *Manager) handleAdminClientKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"clientKey": m.cfg.ClientAPIKey})
}

func (m *Manager) handleAdminConfigYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := yaml.Marshal(m.runtimeConfigYAML())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_yaml_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (m *Manager) runtimeConfigYAML() RuntimeConfigYAML {
	cfg := m.cfg
	db := m.store.Snapshot()
	return RuntimeConfigYAML{
		Version: Version,
		Manager: ManagerConfigYAML{
			Listen:    cfg.Addr,
			PublicURL: cfg.ExternalURL,
		},
		Storage: StorageConfigYAML{
			Mode:        cfg.StorageMode,
			Backend:     m.store.BackendName(),
			ArtifactDir: cfg.ArtifactDir,
			SQLitePath:  cfg.DBPath,
			DatabaseURL: redactedConfigValue(cfg.DatabaseURL),
		},
		Auth: AuthConfigYAML{
			AdminToken:            redactedConfigValue(cfg.AdminToken),
			ClientBootstrapKey:    redactedConfigValue(cfg.ClientAPIKey),
			WorkerEnrollmentToken: redactedConfigValue(cfg.WorkerToken),
			EnforceBalance:        cfg.EnforceBalance,
			AllowDevDefaults:      cfg.AllowDevTokens,
		},
		Scheduler: SchedulerConfigYAML{
			QueueLimit:                          cfg.QueueLimit,
			StreamWaitSeconds:                   int64(cfg.StreamWait.Seconds()),
			AutoBalanceScaleDownCooldownSeconds: int64(cfg.AutoBalanceScaleDownCooldown.Seconds()),
			WorkerHeartbeatSeconds:              int64(cfg.WorkerHeartbeatTimeout.Seconds()),
			WorkerFlap: WorkerFlapConfigYAML{
				Threshold:       cfg.WorkerFlapThreshold,
				WindowSeconds:   int64(cfg.WorkerFlapWindow.Seconds()),
				CooldownSeconds: int64(cfg.WorkerFlapCooldown.Seconds()),
			},
			Quarantine: QuarantineConfigYAML{
				Threshold: cfg.QuarantineThreshold,
				Seconds:   int64(cfg.QuarantineDuration.Seconds()),
			},
		},
		Workers: WorkersConfigYAML{
			Connection:  "outboundWebSocket",
			AutoApprove: cfg.AutoApprove,
			P2P:         workerP2PConfigYAML(db),
		},
		Observability: ObservabilityConfigYAML{
			DashboardStateStream: "websocket",
		},
	}
}

func workerP2PConfigYAML(db Database) WorkerP2PConfigYAML {
	if !db.Settings.P2PEnabled {
		return WorkerP2PConfigYAML{Enabled: false}
	}
	ports := uniqueWorkerP2PPorts(db)
	maxUploads := uniqueWorkerP2PMaxUploads(db)
	timeouts := uniqueWorkerP2PTimeouts(db)
	return WorkerP2PConfigYAML{
		Enabled:                         true,
		Mode:                            "bestEffortPublicBitTorrent",
		Discovery:                       "publicDHTAndMagnet",
		DefaultPort:                     defaultWorkerP2PPort,
		DefaultMaxUploads:               defaultWorkerP2PMaxUploads,
		DefaultDownloadTimeoutSeconds:   int64(defaultWorkerP2PDownloadTimeout.Seconds()),
		AvailableWorkers:                countWorkerP2PAvailable(db),
		ReportingWorkers:                countWorkerP2PReporting(db),
		EffectivePorts:                  ports,
		EffectiveMaxUploads:             maxUploads,
		EffectiveDownloadTimeoutSeconds: timeouts,
	}
}

func countWorkerP2PAvailable(db Database) int {
	count := 0
	for _, node := range db.Nodes {
		if node.P2P != nil && node.P2P.Available {
			count++
		}
	}
	return count
}

func countWorkerP2PReporting(db Database) int {
	count := 0
	for _, node := range db.Nodes {
		if node.P2P != nil {
			count++
		}
	}
	return count
}

func uniqueWorkerP2PPorts(db Database) []int {
	set := map[int]bool{}
	for _, node := range db.Nodes {
		if node.P2P != nil && node.P2P.Port > 0 {
			set[node.P2P.Port] = true
		}
	}
	return sortedIntKeys(set)
}

func uniqueWorkerP2PMaxUploads(db Database) []int {
	set := map[int]bool{}
	for _, node := range db.Nodes {
		if node.P2P != nil && node.P2P.MaxUploads > 0 {
			set[node.P2P.MaxUploads] = true
		}
	}
	return sortedIntKeys(set)
}

func uniqueWorkerP2PTimeouts(db Database) []int64 {
	set := map[int64]bool{}
	for _, node := range db.Nodes {
		if node.P2P != nil && node.P2P.DownloadTimeoutSeconds > 0 {
			set[node.P2P.DownloadTimeoutSeconds] = true
		}
	}
	out := make([]int64, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func sortedIntKeys(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func redactedConfigValue(value string) string {
	if value == "" {
		return "<unset>"
	}
	return "<redacted: configured>"
}
