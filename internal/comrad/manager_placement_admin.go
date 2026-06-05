package comrad

import "net/http"

func (m *Manager) handleAdminPlacementExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, ExplainPlacementWithConfig(m.store.Snapshot(), m.cfg))
}
