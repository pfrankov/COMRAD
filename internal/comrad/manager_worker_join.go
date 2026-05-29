package comrad

import (
	"net/http"
	"strings"
)

func (m *Manager) handleAdminWorkerJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	managerURL := strings.TrimRight(m.baseURL(r), "/")
	writeJSON(w, http.StatusOK, WorkerJoinResponse{
		ManagerURL:     managerURL,
		WorkerToken:    m.cfg.WorkerToken,
		InstallCommand: workerJoinInstallCommand(managerURL, m.cfg.WorkerToken),
	})
}

func workerJoinInstallCommand(managerURL, workerToken string) string {
	return "cd dist/bundle-darwin-arm64\n" +
		"COMRAD_MANAGER_URL=" + shellQuote(managerURL) + " \\\n" +
		"COMRAD_WORKER_TOKEN=" + shellQuote(workerToken) + " \\\n" +
		"scripts/install-worker-macos.sh"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
