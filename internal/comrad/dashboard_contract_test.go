package comrad

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardUsesAdminStateWebSocketInsteadOfPolling(t *testing.T) {
	app := readDashboardSource(t, "web/dashboard/src/App.tsx")
	assertSourceContainsAll(t, app, "new WebSocket", "/api/admin/state/ws", "/api/admin/state/ws-ticket", `url.searchParams.set("ticket", ticket)`)
	if strings.Contains(app, `url.searchParams.set("admin_token"`) {
		t.Fatal("dashboard must not put the admin token into the WebSocket URL")
	}
	if strings.Contains(app, "admin_token") {
		t.Fatal("dashboard must not read the admin token from the URL")
	}
	if strings.Contains(app, "setInterval") {
		t.Fatal("dashboard must not poll /api/admin/state with setInterval")
	}
}

func TestDashboardShowsAdminStateConnectionStatus(t *testing.T) {
	app := readDashboardSource(t, "web/dashboard/src/App.tsx")
	assertSourceContainsAll(t, app, "connectionStatus", "shell.connection.reconnecting", "shell.connection.connected", "onopen", "onclose")
}

func TestDashboardTasksPageUsesLiveStateForDefaultHistory(t *testing.T) {
	tasks := readDashboardSource(t, "web/dashboard/src/pages/tasks.tsx")
	assertSourceContainsAll(t, tasks, "usesLiveState", "serverStateVersion", "state.tasks ?? []", "page?.items ?? []")
	if strings.Contains(tasks, "page?.items ?? state.tasks") {
		t.Fatal("default request history must not pin a stale server page over WebSocket state")
	}
	if strings.Contains(tasks, "page?.summary ?? state.taskSummary") {
		t.Fatal("default request summary must not pin a stale server page over WebSocket state")
	}
}

func TestDashboardKeepsAdminTokenInSettings(t *testing.T) {
	app := readDashboardSource(t, "web/dashboard/src/App.tsx")
	settings := readDashboardSource(t, "web/dashboard/src/pages/settings.tsx")
	if strings.Contains(app, `id="admin-token"`) {
		t.Fatal("admin token input should not live in the global header")
	}
	assertSourceContainsAll(t, settings, "AdminTokenCard", `id="admin-token"`)
	if strings.Contains(settings, "admin_token") {
		t.Fatal("settings must not put the admin token into URLs")
	}
	assertSourceContainsAll(t, app, "active === \"settings\"", "saveAdminToken={saveAdminToken}")
}

func TestDashboardUploadProgressContract(t *testing.T) {
	actions := readDashboardSource(t, "web/dashboard/src/comrad/actions.ts")
	profiles := readDashboardSource(t, "web/dashboard/src/pages/profiles.tsx")
	assertSourceContainsAll(t, actions, "XMLHttpRequest", "onUploadProgress", "upload.onprogress")
	assertSourceContainsAll(t, profiles, "Progress", "profiles.uploadProgress.percent", "profiles.uploadProgress.speed")
}

func TestDashboardConfirmClosesBeforeRunningAction(t *testing.T) {
	app := readDashboardSource(t, "web/dashboard/src/App.tsx")
	assertSourceContainsAll(t, app, "const action = confirm", "setConfirm(null)", "void action?.run()")
	if strings.Contains(app, "finally(() => setConfirm(null))") {
		t.Fatal("confirm dialog should close before long-running actions start")
	}
}

func TestDashboardDetailsOpenInDialogs(t *testing.T) {
	users := readDashboardSource(t, "web/dashboard/src/pages/users.tsx")
	profiles := readDashboardSource(t, "web/dashboard/src/pages/profiles.tsx")
	nodes := readDashboardSource(t, "web/dashboard/src/pages/nodes.tsx")
	if strings.Contains(users, "xl:grid-cols-[minmax(0,1.2fr)_minmax(360px,0.8fr)]") {
		t.Fatal("API client details should not render as a side panel")
	}
	if strings.Contains(profiles, "expandedId") || strings.Contains(nodes, "<details") {
		t.Fatal("similar detail surfaces should use dialogs, not inline expansion")
	}
	assertSourceContainsAll(t, users, "UserDetailDialog", "openDetails")
	assertSourceContainsAll(t, profiles, "ModelTechnicalDetailsDialog")
	assertSourceContainsAll(t, nodes, "NodeTechnicalDetailsDialog")
}

func readDashboardSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertSourceContainsAll(t *testing.T, source string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(source, needle) {
			t.Fatalf("source missing %q", needle)
		}
	}
}
