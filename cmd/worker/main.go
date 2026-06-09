package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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
		P2PPort:                envInt("COMRAD_WORKER_P2P_PORT", 6881),
		P2PMaxUploads:          envInt("COMRAD_WORKER_P2P_MAX_UPLOADS", 8),
		P2PDownloadTimeout:     time.Duration(envInt("COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS", 120)) * time.Second,
		DisableP2P:             envBool("COMRAD_WORKER_DISABLE_P2P", false),
		EnableSelfUpdate:       envBool("COMRAD_ENABLE_SELF_UPDATE", false),
	}
	worker, err := comrad.NewWorker(cfg)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Exit when stdin is closed — the tray app holds the write end of a pipe
	// as our stdin; when it dies (even via SIGKILL) the pipe closes and we exit.
	go func() {
		io.Copy(io.Discard, os.Stdin)
		stop()
	}()

	log.Printf("comrad worker %s connecting to %s", comrad.Version, cfg.ManagerURL)
	statusAddr := os.Getenv("COMRAD_WORKER_STATUS_ADDR")
	if statusAddr != "" {
		go func() {
			if err := worker.ServeStatus(ctx, statusAddr); err != nil {
				log.Printf("status server error: %v", err)
			}
		}()
	}
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
