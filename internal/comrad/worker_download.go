package comrad

import "context"

func (w *Worker) acquireDownloadSlot(ctx context.Context) (func(), error) {
	tokens := w.downloadLimiter()
	w.updateDownloadPressure(1, 0)
	select {
	case tokens <- struct{}{}:
		w.updateDownloadPressure(-1, 1)
		return func() {
			<-tokens
			w.updateDownloadPressure(0, -1)
		}, nil
	case <-ctx.Done():
		w.updateDownloadPressure(-1, 0)
		return nil, ctx.Err()
	}
}

func (w *Worker) downloadLimiter() chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.initDownloadPressureLocked()
	return w.downloadTokens
}

func (w *Worker) updateDownloadPressure(queuedDelta, activeDelta int) {
	w.mu.Lock()
	w.initDownloadPressureLocked()
	w.downloadPressure.Queued += queuedDelta
	w.downloadPressure.Active += activeDelta
	if w.downloadPressure.Queued < 0 {
		w.downloadPressure.Queued = 0
	}
	if w.downloadPressure.Active < 0 {
		w.downloadPressure.Active = 0
	}
	w.node.DownloadPressure = w.downloadPressure
	w.mu.Unlock()
	w.sendFullState()
}

func (w *Worker) initDownloadPressureLocked() {
	if w.cfg.MaxConcurrentDownloads <= 0 {
		w.cfg.MaxConcurrentDownloads = 1
	}
	if w.downloadTokens == nil {
		w.downloadTokens = make(chan struct{}, w.cfg.MaxConcurrentDownloads)
	}
	w.downloadPressure.MaxConcurrent = w.cfg.MaxConcurrentDownloads
	w.node.DownloadPressure = w.downloadPressure
}
