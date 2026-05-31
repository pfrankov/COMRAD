package comrad

import (
	"net/http"

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
	QueueLimit             int                  `yaml:"queueLimit"`
	StreamWaitSeconds      int64                `yaml:"streamWaitSeconds"`
	WorkerHeartbeatSeconds int64                `yaml:"workerHeartbeatSeconds"`
	Quarantine             QuarantineConfigYAML `yaml:"quarantine"`
}

type QuarantineConfigYAML struct {
	Threshold int   `yaml:"threshold"`
	Seconds   int64 `yaml:"seconds"`
}

type WorkersConfigYAML struct {
	Connection  string `yaml:"connection"`
	AutoApprove bool   `yaml:"autoApprove"`
}

type ObservabilityConfigYAML struct {
	DashboardStateStream string `yaml:"dashboardStateStream"`
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
			QueueLimit:             cfg.QueueLimit,
			StreamWaitSeconds:      int64(cfg.StreamWait.Seconds()),
			WorkerHeartbeatSeconds: int64(cfg.WorkerHeartbeatTimeout.Seconds()),
			Quarantine: QuarantineConfigYAML{
				Threshold: cfg.QuarantineThreshold,
				Seconds:   int64(cfg.QuarantineDuration.Seconds()),
			},
		},
		Workers: WorkersConfigYAML{
			Connection:  "outboundWebSocket",
			AutoApprove: cfg.AutoApprove,
		},
		Observability: ObservabilityConfigYAML{
			DashboardStateStream: "websocket",
		},
	}
}

func redactedConfigValue(value string) string {
	if value == "" {
		return "<unset>"
	}
	return "<redacted: configured>"
}
