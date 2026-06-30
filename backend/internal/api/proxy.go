package api

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func newAuthServiceProxy(target string) (http.Handler, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, nil
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(parsed)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("auth service proxy error", "error", err, "path", r.URL.Path)
		writeError(w, http.StatusBadGateway, "auth service unavailable")
	}
	return proxy, nil
}

func mountAuthProxy(mux *http.ServeMux, proxy http.Handler) {
	if proxy == nil {
		return
	}

	paths := []string{
		"/api/auth/login",
		"/api/auth/logout",
		"/api/auth/register",
		"/api/me",
		"/api/companies",
		"/api/users",
		"/api/admin/licenses",
	}
	for _, path := range paths {
		mux.Handle(path, proxy)
	}
	mux.Handle("/api/users/", proxy)
}
