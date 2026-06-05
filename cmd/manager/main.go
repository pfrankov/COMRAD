package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"comrad/internal/comrad"
)

func main() {
	cfg := comrad.ManagerConfig{
		Addr:                         env("COMRAD_MANAGER_ADDR", "127.0.0.1:1922"),
		DBPath:                       env("COMRAD_SQLITE_PATH", env("COMRAD_DB_PATH", "data/comrad.sqlite")),
		DatabaseURL:                  os.Getenv("COMRAD_DATABASE_URL"),
		StorageMode:                  env("COMRAD_STORAGE_MODE", "auto"),
		ArtifactDir:                  env("COMRAD_ARTIFACT_DIR", "data/artifacts"),
		AdminToken:                   os.Getenv("COMRAD_ADMIN_TOKEN"),
		ClientAPIKey:                 os.Getenv("COMRAD_CLIENT_API_KEY"),
		WorkerToken:                  os.Getenv("COMRAD_WORKER_TOKEN"),
		EnforceBalance:               envBoolLocal("COMRAD_ENFORCE_BALANCE", false),
		QueueLimit:                   envInt("COMRAD_QUEUE_LIMIT", 32),
		StreamWait:                   time.Duration(envInt("COMRAD_STREAM_WAIT_SECONDS", 15)) * time.Second,
		AutoBalanceScaleDownCooldown: time.Duration(envInt("COMRAD_AUTO_BALANCE_SCALE_DOWN_COOLDOWN_SECONDS", 300)) * time.Second,
		QuarantineThreshold:          envInt("COMRAD_QUARANTINE_THRESHOLD", 3),
		QuarantineDuration:           time.Duration(envInt("COMRAD_QUARANTINE_SECONDS", 300)) * time.Second,
		WorkerHeartbeatTimeout:       time.Duration(envInt("COMRAD_WORKER_HEARTBEAT_TIMEOUT_SECONDS", 30)) * time.Second,
		WorkerFlapThreshold:          envInt("COMRAD_WORKER_FLAP_THRESHOLD", 4),
		WorkerFlapWindow:             time.Duration(envInt("COMRAD_WORKER_FLAP_WINDOW_SECONDS", 300)) * time.Second,
		WorkerFlapCooldown:           time.Duration(envInt("COMRAD_WORKER_FLAP_COOLDOWN_SECONDS", 300)) * time.Second,
		AutoApprove:                  envBoolLocal("COMRAD_AUTO_APPROVE_WORKERS", true),
		ExternalURL:                  os.Getenv("COMRAD_EXTERNAL_URL"),
		AllowDevTokens:               envBoolLocal("COMRAD_ALLOW_DEV_DEFAULTS", false),
	}
	manager, err := comrad.NewManager(cfg)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("admin token configured: %t", cfg.AdminToken != "")
	log.Printf("client api key configured: %t", cfg.ClientAPIKey != "")
	log.Printf("worker enrollment token configured: %t", cfg.WorkerToken != "")
	if err := manager.Serve(ctx); err != nil {
		log.Fatal(err)
	}
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func envBoolLocal(name string, def bool) bool {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
