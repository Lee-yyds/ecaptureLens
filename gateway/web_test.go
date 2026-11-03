package gateway

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebHandlerAssetsAndBrowserAPI(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	dir := t.TempDir()
	for name, data := range map[string]string{"index.html": "<html>Lens</html>", "main.dart.js": "window.lens = true;"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	h, err := s.WebHandler("secret", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, mime string
		want       int
	}{{"/", "text/html", 200}, {"/main.dart.js", "javascript", 200}, {"/assets/", "", 404}, {"/missing", "", 404}, {"/escape", "", 404}} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8787"+tc.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want || !strings.Contains(w.Header().Get("Content-Type"), tc.mime) {
			t.Fatalf("%s: %d %s %q", tc.path, w.Code, w.Header().Get("Content-Type"), w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("missing response policy: %v", w.Header())
		}
	}
	for _, tc := range []struct {
		origin, token, body string
		secure              bool
		want                int
	}{
		{"http://127.0.0.1:8787", "secret", `{"method":"system.info"}`, false, 200},
		{"", "secret", `{"method":"system.info"}`, false, 200},
		{"http://127.0.0.1:8787", "", `{"method":"system.info"}`, false, 401},
		{"http://evil.test", "secret", `{"method":"system.info"}`, false, 403},
		{"null", "secret", `{"method":"system.info"}`, false, 403},
		{"https://127.0.0.1:8787", "secret", `{"method":"system.info"}`, false, 403},
		{"https://127.0.0.1:8787", "secret", `{"method":"system.info"}`, true, 200},
		{"http://127.0.0.1:8787", "secret", `{"method":"system.info"} {}`, false, 400},
	} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8787/api", bytes.NewBufferString(tc.body))
		r.Header.Set("Origin", tc.origin)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		if tc.secure {
			r.TLS = &tls.ConnectionState{}
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("origin %q secure %v: got %d %s", tc.origin, tc.secure, w.Code, w.Body.String())
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("Web handler enabled cross-origin access")
		}
	}
}

func TestWebHandlerRequiresAssetsAndToken(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.WebHandler("secret", t.TempDir()); err == nil {
		t.Fatal("missing index accepted")
	}
	if _, err := s.WebHandler("", t.TempDir()); err == nil {
		t.Fatal("unauthenticated web handler accepted")
	}
	r := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{"method":"system.info"}`))
	r.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	s.Handler("").ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("API-only handler now allows browsers")
	}
}
