package comrad

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxRuntimeRestartAttempts = 3

type runtimeStreamStats struct {
	FirstTokenAt     *time.Time
	PromptTokens     int
	CompletionTokens int
	GenerationMS     int64
	TokensPerSecond  float64
	HasPromptTokens  bool
	HasGeneration    bool
	HasTokensPerSec  bool
}

type llamaServerProcess struct {
	cmd        *exec.Cmd
	baseURL    string
	profileKey string
	stderr     *cappedBuffer
	done       chan struct{}
	waitErr    error
	waitMu     sync.Mutex
}

type modelSupportArtifact struct {
	Kind string
	Name string
	Path string
}

func (w *Worker) ensureRuntimeServer(ctx context.Context, slotID string, profile WorkloadProfile) error {
	key := assignmentKey(profile)
	if w.runtimeServerReady(slotID, key) {
		return nil
	}
	return w.startRuntimeServerForSlot(ctx, slotID, profile, true)
}

func (w *Worker) restartRuntimeServer(ctx context.Context, slotID string, profile WorkloadProfile) error {
	return w.startRuntimeServerForSlot(ctx, slotID, profile, false)
}

func (w *Worker) startRuntimeServerForSlot(ctx context.Context, slotID string, profile WorkloadProfile, resetRestarts bool) error {
	w.stopRuntimeServer(slotID)
	proc, err := w.startLlamaServer(ctx, profile)
	if err != nil {
		return err
	}
	key := assignmentKey(profile)
	w.mu.Lock()
	w.runtimes[slotID] = proc
	w.warm[key] = profile
	if w.runtimeRestarts == nil {
		w.runtimeRestarts = map[string]int{}
	}
	if resetRestarts {
		w.runtimeRestarts[slotID] = 0
	}
	w.mu.Unlock()
	go w.watchRuntimeServer(slotID, profile, proc)
	return nil
}

func (w *Worker) runtimeServerReady(slotID, profileKey string) bool {
	w.mu.Lock()
	proc := w.runtimes[slotID]
	w.mu.Unlock()
	return proc != nil && proc.profileKey == profileKey && proc.alive()
}

func (p *llamaServerProcess) alive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (w *Worker) startLlamaServer(ctx context.Context, profile WorkloadProfile) (*llamaServerProcess, error) {
	runtimePath, modelPath, support, err := w.runtimeCommandInputs(profile)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		port, err := freeLocalPort()
		if err != nil {
			return nil, err
		}
		args, err := llamaServerArgs(profile, modelPath, support, port)
		if err != nil {
			return nil, err
		}
		proc, err := startLlamaServerProcess(runtimePath, args, port, assignmentKey(profile))
		if err != nil {
			return nil, err
		}
		if err := waitForLlamaServerReady(ctx, w.client, proc, w.cfg.RuntimeStartWait); err != nil {
			lastErr = err
			stderr := proc.stderr.String()
			proc.stop()
			if llamaPortConflict(err, stderr) {
				continue
			}
			return nil, err
		}
		return proc, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("llama-server did not start")
}

func (w *Worker) runtimeCommandInputs(profile WorkloadProfile) (string, string, []modelSupportArtifact, error) {
	if !isLlamaCppAdapter(profile.RuntimeAdapter) {
		return "", "", nil, fmt.Errorf("unsupported runtime adapter %q", profile.RuntimeAdapter)
	}
	runtimePath := llamaServerExecutablePath(w.cfg)
	if runtimePath == "" {
		return "", "", nil, errors.New("llama-server executable is not installed on this Worker")
	}
	modelPath, support := w.llamaModelArtifacts(profile)
	if modelPath == "" {
		return "", "", nil, errors.New("llama.cpp model artifact is required")
	}
	return runtimePath, modelPath, support, nil
}

func startLlamaServerProcess(runtimePath string, args []string, port int, profileKey string) (*llamaServerProcess, error) {
	runtimeDir := filepath.Dir(runtimePath)
	_ = os.Chmod(runtimePath, 0o755)
	cmd := exec.Command(runtimePath, args...)
	cmd.Dir = runtimeDir
	cmd.Env = append(os.Environ(), "DYLD_LIBRARY_PATH="+runtimeDir)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("llama-server failed to start: %w", err)
	}
	proc := &llamaServerProcess{cmd: cmd, baseURL: "http://127.0.0.1:" + strconv.Itoa(port), profileKey: profileKey, stderr: &cappedBuffer{limit: 64 * 1024}, done: make(chan struct{})}
	go discardRuntimeOutput(stdout)
	go captureRuntimeOutput(stderr, proc.stderr)
	go func() {
		err := cmd.Wait()
		proc.waitMu.Lock()
		proc.waitErr = err
		proc.waitMu.Unlock()
		close(proc.done)
	}()
	return proc, nil
}

func waitForLlamaServerReady(ctx context.Context, client *http.Client, proc *llamaServerProcess, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if llamaHealthOK(ctx, client, proc.baseURL) {
			return nil
		}
		select {
		case <-proc.done:
			err := proc.err()
			if err == nil {
				err = errors.New("process exited")
			}
			return fmt.Errorf("llama-server exited before ready: %w: %s", err, strings.TrimSpace(proc.stderr.String()))
		case <-deadline.C:
			return fmt.Errorf("llama-server did not become ready within %s: %s", timeout, strings.TrimSpace(proc.stderr.String()))
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *llamaServerProcess) err() error {
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	return p.waitErr
}

func (p *llamaServerProcess) failureText() string {
	if p == nil {
		return ""
	}
	parts := []string{}
	if err := p.err(); err != nil {
		parts = append(parts, err.Error())
	}
	if stderr := strings.TrimSpace(p.stderr.String()); stderr != "" {
		parts = append(parts, stderr)
	}
	return strings.Join(parts, ": ")
}

func llamaHealthOK(ctx context.Context, client *http.Client, baseURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (p *llamaServerProcess) stop() {
	if p == nil {
		return
	}
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(os.Interrupt)
	}
	select {
	case <-p.done:
	case <-time.After(time.Second):
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		select {
		case <-p.done:
		case <-time.After(time.Second):
		}
	}
}

func (w *Worker) stopRuntimeServer(slotID string) {
	w.mu.Lock()
	proc := w.runtimes[slotID]
	delete(w.runtimes, slotID)
	w.mu.Unlock()
	proc.stop()
}

func (w *Worker) stopAllRuntimeServers() {
	w.mu.Lock()
	procs := make([]*llamaServerProcess, 0, len(w.runtimes))
	for slotID, proc := range w.runtimes {
		procs = append(procs, proc)
		delete(w.runtimes, slotID)
	}
	w.mu.Unlock()
	for _, proc := range procs {
		proc.stop()
	}
}

func (w *Worker) watchRuntimeServer(slotID string, profile WorkloadProfile, proc *llamaServerProcess) {
	<-proc.done
	if !w.claimExitedRuntime(slotID, proc) {
		return
	}
	if !w.reserveRuntimeRestart(slotID) {
		w.setSlotState(slotID, SlotStateError, profile.ID, "llama-server exited too often: "+proc.failureText())
		return
	}
	w.setSlotState(slotID, SlotStateWarming, profile.ID, "llama-server exited; restarting")
	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.RuntimeStartWait)
	defer cancel()
	if err := w.restartRuntimeServer(ctx, slotID, profile); err != nil {
		w.setSlotState(slotID, SlotStateError, profile.ID, err.Error())
		return
	}
	w.setSlotState(slotID, SlotStateReady, profile.ID, "")
}

func (w *Worker) claimExitedRuntime(slotID string, proc *llamaServerProcess) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.runtimes[slotID] != proc {
		return false
	}
	if w.slots[slotID].State == SlotStateServing {
		return false
	}
	delete(w.runtimes, slotID)
	return true
}

func (w *Worker) reserveRuntimeRestart(slotID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.runtimeRestarts == nil {
		w.runtimeRestarts = map[string]int{}
	}
	if w.runtimeRestarts[slotID] >= maxRuntimeRestartAttempts {
		return false
	}
	w.runtimeRestarts[slotID]++
	return true
}

func (w *Worker) runtimeServerHealthy(slotID, profileKey string) bool {
	w.mu.Lock()
	proc := w.runtimes[slotID]
	w.mu.Unlock()
	if proc == nil || proc.profileKey != profileKey || !proc.alive() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return llamaHealthOK(ctx, w.client, proc.baseURL)
}

func (w *Worker) recoverRuntimeSlot(slotID string, profile WorkloadProfile, reason string) {
	key := assignmentKey(profile)
	if w.runtimeServerHealthy(slotID, key) {
		w.setSlotState(slotID, SlotStateReady, profile.ID, "")
		return
	}
	if !w.reserveRuntimeRestart(slotID) {
		w.stopRuntimeServer(slotID)
		w.setSlotState(slotID, SlotStateError, profile.ID, "llama-server restart limit reached")
		return
	}
	w.setSlotState(slotID, SlotStateWarming, profile.ID, reason)
	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.RuntimeStartWait)
	defer cancel()
	if err := w.restartRuntimeServer(ctx, slotID, profile); err != nil {
		w.setSlotState(slotID, SlotStateError, profile.ID, err.Error())
		return
	}
	w.setSlotState(slotID, SlotStateReady, profile.ID, "")
}

func freeLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func llamaServerExecutablePath(cfg WorkerConfig) string {
	if cfg.LlamaServerPath != "" {
		return executableFilePath(cfg.LlamaServerPath)
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return executableFilePath(filepath.Join(filepath.Dir(exe), "llama-server"))
}

func executableFilePath(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return ""
	}
	return path
}

func llamaServerArgs(profile WorkloadProfile, modelPath string, support []modelSupportArtifact, port int) ([]string, error) {
	args := []string{"--host", "127.0.0.1", "--port", strconv.Itoa(port), "--model", modelPath}
	if profile.LLM != nil && profile.LLM.ContextTokens > 0 {
		args = append(args, "--ctx-size", strconv.Itoa(profile.LLM.ContextTokens))
	}
	profileArgs := profile.Runtime.LlamaCpp.Args
	if err := validateLlamaServerProfileArgs(profileArgs); err != nil {
		return nil, err
	}
	if path := firstMMProjPath(support); path != "" && !hasRuntimeArg(profileArgs, "--mmproj") {
		args = append(args, "--mmproj", path)
	}
	args = append(args, profileArgs...)
	return args, nil
}

func validateLlamaServerProfileArgs(args []string) error {
	for _, arg := range args {
		name := llamaServerArgName(arg)
		if isManagedLlamaServerArg(name) {
			return fmt.Errorf("llama.cpp server arg %q is managed by COMRAD", name)
		}
	}
	return nil
}

func llamaServerArgName(arg string) string {
	if i := strings.IndexByte(arg, '='); i >= 0 {
		return arg[:i]
	}
	return arg
}

func isManagedLlamaServerArg(arg string) bool {
	switch arg {
	case "--host", "--port", "--model", "-m", "--mmproj", "--ctx-size", "-c", "--api-key", "--api-key-file", "--ssl-key-file", "--ssl-cert-file":
		return true
	default:
		return false
	}
}

func llamaPortConflict(err error, stderr string) bool {
	text := strings.ToLower(err.Error() + " " + stderr)
	return strings.Contains(text, "address already in use") || strings.Contains(text, "bind: address")
}

func discardRuntimeOutput(r io.Reader) {
	_, _ = io.Copy(io.Discard, r)
}

func captureRuntimeOutput(r io.Reader, dst io.Writer) {
	_, _ = io.Copy(dst, r)
}

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 || b.Len() < b.limit {
		keep := len(p)
		if b.limit > 0 && b.Len()+keep > b.limit {
			keep = b.limit - b.Len()
		}
		if keep > 0 {
			_, _ = b.Buffer.Write(p[:keep])
		}
	}
	return len(p), nil
}
