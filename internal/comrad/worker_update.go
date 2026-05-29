package comrad

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (w *Worker) handleUpdate(payload UpdatePayload) {
	if payload.URL == "" {
		return
	}
	for {
		w.mu.Lock()
		active := len(w.active)
		w.mu.Unlock()
		if active == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	spec := ArtifactSpec{ID: payload.Update.ArtifactID, Name: payload.Update.ArtifactID, SHA256: payload.Update.SHA256, URL: payload.URL, Kind: "worker_update"}
	if err := w.ensureArtifact(context.Background(), spec); err != nil {
		w.sendUpdateTelemetry(payload.Update.ID, "update_failed", err.Error())
		return
	}
	w.mu.Lock()
	path := w.cache[payload.Update.ArtifactID]
	w.mu.Unlock()
	if err := w.verifySignature(path, payload.Update); err != nil {
		w.sendUpdateTelemetry(payload.Update.ID, "update_failed", err.Error())
		return
	}
	if !w.cfg.EnableSelfUpdate {
		w.sendUpdateTelemetry(payload.Update.ID, "update_verified", "self update disabled; verified package is cached")
		return
	}
	if err := installAndRestart(path); err != nil {
		w.sendUpdateTelemetry(payload.Update.ID, "update_failed", err.Error())
		return
	}
}

func (w *Worker) verifySignature(path string, update UpdateRecord) error {
	if update.Signature == "" && update.PublicKey == "" {
		return nil
	}
	if update.Signature == "" || update.PublicKey == "" {
		return errors.New("signature and publicKey must be provided together")
	}
	pub, err := base64.StdEncoding.DecodeString(update.PublicKey)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(update.Signature)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), b, sig) {
		return errors.New("update signature verification failed")
	}
	return nil
}

func (w *Worker) sendUpdateTelemetry(updateID, status, detail string) {
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgTelemetry, NodeID: w.node.ID, Payload: MarshalPayload(map[string]string{"updateId": updateID, "status": status, "detail": detail})})
}

func installAndRestart(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	next := exe + ".next"
	if err := copyFile(path, next, 0o755); err != nil {
		return err
	}
	backup := exe + ".old"
	_ = os.Remove(backup)
	_ = os.Rename(exe, backup)
	if err := os.Rename(next, exe); err != nil {
		_ = os.Rename(backup, exe)
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, in); err != nil {
		return err
	}
	return os.WriteFile(dst, buf.Bytes(), mode)
}

func (w *Worker) saveState() error {
	w.mu.Lock()
	state := workerStateFile{NodeID: w.node.ID, NodeToken: w.nodeToken, Cache: map[string]string{}}
	for k, v := range w.cache {
		state.Cache[k] = v
	}
	for id := range w.warm {
		state.WarmProfile = append(state.WarmProfile, id)
	}
	w.mu.Unlock()
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := EnsureDir(filepath.Dir(w.cfg.StatePath)); err != nil {
		return err
	}
	return os.WriteFile(w.cfg.StatePath, b, 0o600)
}

func safeArtifactFileName(id string) string {
	return strings.NewReplacer("/", "_", ":", "_").Replace(id)
}
