package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"comrad/internal/comrad"
)

func main() {
	cfg := comrad.WorkerConfig{
		ManagerURL:             env("COMRAD_MANAGER_URL", "http://127.0.0.1:1922"),
		Token:                  env("COMRAD_WORKER_TOKEN", "dev-worker-token"),
		NodeID:                 os.Getenv("COMRAD_NODE_ID"),
		Name:                   os.Getenv("COMRAD_NODE_NAME"),
		StatePath:              env("COMRAD_WORKER_STATE_PATH", "data/worker-state.json"),
		CacheDir:               env("COMRAD_WORKER_CACHE_DIR", "data/worker-cache"),
		SlotCount:              envInt("COMRAD_WORKER_SLOTS", 1),
		MaxConcurrentDownloads: envInt("COMRAD_WORKER_MAX_CONCURRENT_DOWNLOADS", 1),
		RAMBytes:               envInt64("COMRAD_WORKER_RAM_BYTES", 8<<30),
		VRAMBytes:              envInt64("COMRAD_WORKER_VRAM_BYTES", 0),
		UnifiedBytes:           envInt64("COMRAD_WORKER_UNIFIED_BYTES", 8<<30),
		DiskBytes:              envInt64("COMRAD_WORKER_DISK_BYTES", 20<<30),
		EnableSelfUpdate:       envBool("COMRAD_ENABLE_SELF_UPDATE", false),
	}
	worker, err := comrad.NewWorker(cfg)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("comrad worker %s connecting to %s", comrad.Version, cfg.ManagerURL)
	if err := worker.Run(ctx); err != nil {
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

func envInt64(name string, def int64) int64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return i
}

func envBool(name string, def bool) bool {
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
