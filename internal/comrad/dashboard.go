package comrad

import (
	"embed"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

//go:embed dashboard_static/* dashboard_static/assets/*
var dashboardAssets embed.FS

const dashboardSystemLocaleMarker = `window.__COMRAD_SYSTEM_LOCALE__ = ""`

func (m *Manager) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/dashboard/assets/") {
		serveDashboardAsset(w, r)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/dashboard/" {
		http.NotFound(w, r)
		return
	}
	index, err := dashboardAssets.ReadFile("dashboard_static/index.html")
	if err != nil {
		http.Error(w, "dashboard assets missing", http.StatusInternalServerError)
		return
	}
	index = injectDashboardSystemLocale(index, dashboardLocaleFromRequest(r))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setDashboardCacheHeaders(w)
	_, _ = w.Write(index)
}

func injectDashboardSystemLocale(index []byte, locale string) []byte {
	if locale == "" {
		return index
	}
	next := strings.Replace(string(index), dashboardSystemLocaleMarker, `window.__COMRAD_SYSTEM_LOCALE__ = `+strconv.Quote(locale), 1)
	return []byte(next)
}

func dashboardLocaleFromRequest(r *http.Request) string {
	return matchDashboardLocale(r.Header.Get("Accept-Language"))
}

func matchDashboardLocale(header string) string {
	bestLocale := ""
	bestQ := -1.0
	for _, part := range strings.Split(header, ",") {
		tag, q := parseAcceptLanguagePart(part)
		locale := normalizeDashboardLocale(tag)
		if locale != "" && q > bestQ {
			bestLocale = locale
			bestQ = q
		}
	}
	return bestLocale
}

func parseAcceptLanguagePart(part string) (string, float64) {
	fields := strings.Split(part, ";")
	tag := strings.TrimSpace(fields[0])
	q := 1.0
	for _, field := range fields[1:] {
		field = strings.TrimSpace(field)
		if strings.HasPrefix(field, "q=") {
			if parsed, err := strconv.ParseFloat(strings.TrimPrefix(field, "q="), 64); err == nil {
				q = parsed
			}
		}
	}
	return tag, q
}

func normalizeDashboardLocale(tag string) string {
	value := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
	base := strings.Split(value, "-")[0]
	if dashboardLocaleSupported(base) {
		return base
	}
	return ""
}

func dashboardLocaleSupported(locale string) bool {
	switch locale {
	case "en", "zh", "es", "fr", "ru", "de", "ja", "pt":
		return true
	default:
		return false
	}
}

func serveDashboardAsset(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(dashboardAssets, "dashboard_static")
	if err != nil {
		http.Error(w, "dashboard assets missing", http.StatusInternalServerError)
		return
	}
	setDashboardCacheHeaders(w)
	http.StripPrefix("/dashboard/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}

func setDashboardCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache")
}
