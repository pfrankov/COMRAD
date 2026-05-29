package comrad

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionDeployContract(t *testing.T) {
	root := repoRoot(t)
	assertFileContains(t, root, "Makefile", "comrad-manager-linux-amd64", "deploy-production-manager")
	assertFileContains(t, root, "Makefile", "dashboard-build", "web/dashboard")
	assertFileContains(t, root, "Makefile", "go-test-packages")
	assertFileNotContains(t, root, "Makefile", "| xargs $(GO) test")
	assertFileContains(t, root, "Dockerfile", "comrad-manager", "COMRAD_MANAGER_ADDR=0.0.0.0:8080")
	assertFileContains(t, root, "compose.yaml", "manager:", "postgres:", "prometheus:", "COMRAD_DATABASE_URL", "COMRAD_STORAGE_MODE:", "auto", "./imports:/var/lib/comrad/imports:ro")
	assertFileContains(t, root, ".gitignore", "imports/")
	assertFileContains(t, root, ".dockerignore", ".env", ".env.*", ".agents/", ".tools/", "internal/comrad/dashboard_static/", "imports/")
	assertFileContains(t, root, "README.md", "docker ps", "Docker daemon", "/var/lib/comrad/imports")
	assertFileContains(t, root, "docs/operations.md", "docker ps", "Docker daemon", "/var/lib/comrad/imports")
	assertFileContains(t, root, "docs/model-management.md", "/var/lib/comrad/imports")
	assertFileContains(t, root, "skills/comrad/SKILL.md", "docker ps", "Docker daemon", "/var/lib/comrad/imports")
	assertFileContains(t, root, "deploy/prometheus/prometheus.yml", "comrad-manager", "/metrics")
	assertFileContains(t, root, "scripts/deploy-manager-debian.sh", "make validate", "/health", "/ready", "rollback", "sha256")
	assertFileContains(t, root, "scripts/rollback-local.sh", "bin/comrad-manager", "bin/comrad-worker", "bin/llama-server")
	assertFileContains(t, root, "Makefile", "bundle-llama-macos.sh", "sign-macos-bundle.sh", "llama-runtime.env", "$(DIST)/bundle-darwin-arm64/bin/llama-server")
	assertFileContains(t, root, "scripts/llama-runtime.env", "DEFAULT_LLAMA_CPP_URL", "DEFAULT_LLAMA_CPP_SHA256")
	assertFileContains(t, root, "scripts/bundle-llama-macos.sh", "llama-runtime.env", "copy_runtime_neighbors", "llama-server")
	assertFileContains(t, root, "scripts/sign-macos-bundle.sh", "codesign", "Ad-hoc signed macOS bundle")
	assertFileContains(t, root, "scripts/install-worker-macos.sh", "COMRAD_WORKER_UNIFIED_BYTES", "COMRAD_WORKER_DISK_BYTES", "COMRAD_LLAMA_CPP_URL", "COMRAD_LLAMA_CPP_SHA256", "download_llama_cpp_server", "sign_installed_binaries", "verify_llama_server", "llama-server")
	assertFileNotContains(t, root, "scripts/install-worker-macos.sh", "COMRAD_LLAMA_CPP_ENABLED", "COMRAD_LLAMA_CPP_PATH", "llama-cli")
	assertFileContains(t, root, "internal/comrad/worker_runtime.go", "llama-server", "/health")
	assertFileContains(t, root, "internal/comrad/worker_runtime_stream.go", "/v1/chat/completions", "text/event-stream")
	assertFileNotContains(t, root, "cmd/worker/main.go", "COMRAD_LLAMA_CPP_PATH", "LlamaCppPath")
	assertFileContains(t, root, "internal/comrad/manager.go", "/api/admin/worker-join")
	assertFileContains(t, root, "internal/comrad/manager_openapi.go", "/api/admin/worker-join", "WorkerJoinResponse")
	assertFileContains(t, root, "AGENTS.md", "All implementation work must start from tests", "Do not patch deployed machines manually")
	assertFileContains(t, root, "web/dashboard/components.json", "ui.shadcn.com", "tsx")
	assertFileContains(t, root, "web/dashboard/src/components/ui/card.tsx", "Card")
	assertFileContains(t, root, "README.md", "The macOS Worker bundle includes `bin/llama-server`")
	assertFileContains(t, root, "docs/operations.md", "downloads the pinned llama.cpp macOS arm64 archive")
	assertFileContains(t, root, "docs/model-management.md", "The macOS bundle includes `llama-server`")
	assertFileContains(t, root, "skills/comrad/SKILL.md", "The macOS bundle includes `bin/llama-server`")
}

func TestManagerDefaultPortContract(t *testing.T) {
	root := repoRoot(t)
	assertFileContains(t, root, "compose.yaml", "${COMRAD_MANAGER_PORT:-1922}:8080")
	assertFileContains(t, root, "scripts/deploy-manager-debian.sh", `PORT="${COMRAD_MANAGER_PORT:-1922}"`)
	assertFileContains(t, root, "scripts/run-local-manager.sh", "127.0.0.1:1922")
	assertFileContains(t, root, "scripts/run-local-worker.sh", "http://127.0.0.1:1922")
	assertFileContains(t, root, "scripts/install-worker-macos.sh", "http://127.0.0.1:1922")
	assertFileContains(t, root, "scripts/smoke-local.sh", "http://127.0.0.1:1922")
	assertFileContains(t, root, "scripts/check-network.sh", "http://127.0.0.1:1922")
	assertFileContains(t, root, "scripts/e2e-real-llama.sh", `COMRAD_E2E_PORT:-1922`)
	assertFileContains(t, root, "cmd/manager/main.go", "127.0.0.1:1922")
	assertFileContains(t, root, "cmd/worker/main.go", "http://127.0.0.1:1922")
	assertFileContains(t, root, "internal/comrad/worker.go", "http://127.0.0.1:1922")
	assertFileContains(t, root, "web/dashboard/vite.config.ts", "http://127.0.0.1:1922")
	assertFileContains(t, root, "README.md", "COMRAD_MANAGER_PORT` or `1922")
	assertFileContains(t, root, "docs/operations.md", "http://<manager-host>:1922")
	assertFileContains(t, root, "skills/comrad/SKILL.md", "BASE_URL='http://127.0.0.1:1922'")
}

func TestDashboardDesignContract(t *testing.T) {
	root := repoRoot(t)
	assertFileContains(t, root, "web/dashboard/src/index.css", "--background: #fafafa", "--foreground: #171717", "--card: #ffffff", "--border: #ebebeb", `--font-sans: "Geist Variable"`, `"Geist Mono Variable"`, "--shadow-card:", "--status-ready-bg:", ".dark")
	assertFileContains(t, root, "web/dashboard/src/index.css", ".sidebar-fade", `.sidebar-fade[data-overflow="true"]`, "mask-image: linear-gradient", "-webkit-mask-image")
	assertFileContains(t, root, "web/dashboard/src/components/ui/card.tsx", "rounded-lg bg-card", "shadow-card")
	assertFileContains(t, root, "web/dashboard/src/components/ui/toggle.tsx", "border-transparent", "data-[state=on]:bg-card", "data-[state=on]:shadow-card-soft")
	assertFileContains(t, root, "web/dashboard/src/components/comrad/status-badge.tsx", "bg-status-ready-bg", "text-status-failed")
	assertFileContains(t, root, "web/dashboard/src/components/theme-provider.tsx", "prefers-color-scheme: dark", "localStorage.setItem", "storageKey", "resolvedTheme")
	assertFileContains(t, root, "web/dashboard/index.html", "comrad.theme", "prefers-color-scheme: dark", `: "system"`, "__COMRAD_SYSTEM_LOCALE__")
	assertFileContains(t, root, "web/dashboard/src/main.tsx", "ThemeProvider", `defaultTheme="system"`, `storageKey="comrad.theme"`)
	assertFileContains(t, root, "web/dashboard/src/App.tsx", "SidebarText", "scrollWidth", "clientWidth", "data-overflow", `title={section.description}`)
	assertFileContains(t, root, "web/dashboard/src/App.tsx", "/api/admin/state/ws", "new WebSocket", `active === "settings"`)
	assertFileContains(t, root, "web/dashboard/src/pages/settings.tsx", `shell.saveAdminToken`, `id="admin-token"`, "AdminTokenCard")
	assertFileContains(t, root, "web/dashboard/src/App.tsx", `active === "settings"`, "state={{} as StateResponse}")
	assertFileNotContains(t, root, "web/dashboard/src/App.tsx", "shell.operatorPath")
	assertFileContains(t, root, "web/dashboard/src/pages/profiles.tsx", "Edit model", "Linked model files")
	assertFileNotContains(t, root, "web/dashboard/src/pages/profiles.tsx", "Rename or tune", "Supporting model files", "Manager-local GGUF path", "Or use a Manager-local")
	assertFileContains(t, root, "web/dashboard/src/pages/users.tsx", "API clients", "Find an API client", "Lookup key", "Edit client", "Top up balance", "Issue API key")
	assertFileContains(t, root, "internal/comrad/manager_accounting_admin.go", "handleAdminAPIKeyLookup", "handleAdminUserUpdate")
	assertFileContains(t, root, "web/dashboard/src/pages/updates.tsx", "Worker software updates", "Model edits do not use updates")
	assertFileNotContains(t, root, "web/dashboard/src/pages/nodes.tsx", "CpuIcon", `label="Version"`)
	assertFileContains(t, root, "web/dashboard/src/pages/settings.tsx", "ThemeCard", "theme.system", "theme.light", "theme.dark", "setTheme")
	assertFileNotContains(t, root, "web/dashboard/src/pages/settings.tsx", "sidebar-fade")
	assertFileNotContains(t, root, "web/dashboard/src/App.tsx", "dark min-h-svh")
	assertFileNotContains(t, root, "web/dashboard/src/main.tsx", `defaultTheme="dark"`)
}

func TestDashboardInternationalizationContract(t *testing.T) {
	root := repoRoot(t)
	assertFileContains(t, root, "web/dashboard/package.json", "i18n:validate", "validate-i18n.mjs")
	assertFileContains(t, root, "web/dashboard/scripts/validate-i18n.mjs", "missing", "unused", "usedKeys", "locales")
	assertFileContains(t, root, "web/dashboard/src/i18n/config.ts", `"en"`, `"zh"`, `"es"`, `"fr"`, `"ru"`, `"de"`, `"ja"`, `"pt"`, "detectLocale", "__COMRAD_SYSTEM_LOCALE__")
	assertFileContainsInOrder(t, root, "web/dashboard/src/i18n/config.ts", "const primary = primaryNavigatorLocale(languages)", "const intl = intlLocale()", "const seeded = systemLocaleSeed()")
	assertFileContainsInOrder(t, root, "web/dashboard/index.html", "const primaryLocale = normalizeLocale", "const intlLocale = normalizeLocale", "const serverLocale = normalizeLocale", ": primaryLocale ||", "intlLocale ||")
	assertFileNotContains(t, root, "web/dashboard/index.html", ".find(Boolean)")
	assertFileContains(t, root, "web/dashboard/src/i18n/i18n-provider.tsx", "comrad.locale", "navigator.languages", "fallback", "document.documentElement.lang")
	assertFileContains(t, root, "web/dashboard/src/main.tsx", "I18nProvider", `defaultLanguage="system"`, `storageKey="comrad.locale"`)
	assertFileContains(t, root, "web/dashboard/src/pages/settings.tsx", "Language", "setLanguage", "resolvedLocale")
	for _, page := range []string{"overview", "tasks", "profiles", "placement", "nodes", "artifacts", "users", "updates"} {
		assertFileContains(t, root, filepath.Join("web/dashboard/src/pages", page+".tsx"), "useI18n")
	}
	assertFileContains(t, root, "web/dashboard/src/i18n/messages/ru.json", "overview.title", "users.title", "profiles.title", "placement.title", "nodes.title", "artifacts.title", "tasks.title", "updates.title")
	assertFileContains(t, root, "README.md", "system detection", "Chinese", "Japanese", "Portuguese", "/api/admin/worker-join")
	assertFileContains(t, root, "docs/dashboard.md", "npm run i18n:validate", "Missing runtime values fall back to English", "unused stale key")
	for _, locale := range []string{"en", "zh", "es", "fr", "ru", "de", "ja", "pt"} {
		assertFileContains(t, root, filepath.Join("web/dashboard/src/i18n/messages", locale+".json"), "settings.language.title")
		assertFileNotContains(t, root, filepath.Join("web/dashboard/src/i18n/messages", locale+".json"), "shell.operatorPath")
	}
	assertFileContains(t, root, "web/dashboard/src/i18n/messages/ru.json", "Панель управления", "Ресурсы", "Администрирование", "Воркеры", "API-клиенты", "ПО воркеров", "Справочник API", "Проверка перед деплоем", "Сохранить токен", "Скорость загрузки")
	assertFileNotContains(t, root, "web/dashboard/src/i18n/messages/ru.json", `"common.refresh"`, "Плоскость управления", "Сервис", "Мощность", "Дрейнить", "разбан", "валидационный gate", "admin-only", "Manager-only", "rollout", "Узлы", "узлы", "среда выполнения", "среды выполнения", "среду выполнения", "runtime", "рантайм")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func assertFileContains(t *testing.T, root, name string, wants ...string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("%s missing %q", name, want)
		}
	}
}

func assertFileNotContains(t *testing.T, root, name string, rejects ...string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, reject := range rejects {
		if strings.Contains(text, reject) {
			t.Fatalf("%s unexpectedly contains %q", name, reject)
		}
	}
}

func assertFileContainsInOrder(t *testing.T, root, name string, wants ...string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	offset := 0
	for _, want := range wants {
		index := strings.Index(text[offset:], want)
		if index < 0 {
			t.Fatalf("%s missing %q after byte %d", name, want, offset)
		}
		offset += index + len(want)
	}
}
