package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutesServesLoginPageForCleanURL(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(webDir, "login.html"),
		[]byte("<!doctype html><title>Login page</title>"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	server := NewServer(nil, nil, nil, nil, webDir)
	request := httptest.NewRequest(http.MethodGet, "/login?next=%2F&reason=required", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Login page") {
		t.Fatalf("expected login page, got %q", response.Body.String())
	}
}

func TestLoginPageRejectsUnsupportedMethods(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, t.TempDir())
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", response.Code)
	}
}
