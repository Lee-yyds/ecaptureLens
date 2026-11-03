package gateway

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
)

// WebHandler serves the Flutter bundle and its authenticated, same-origin API.
// API-only and embedded Android callers continue to use Handler.
func (s *Service) WebHandler(token, directory string) (http.Handler, error) {
	if token == "" {
		return nil, fmt.Errorf("web requires an API token")
	}
	index, err := os.OpenInRoot(directory, "index.html")
	if err != nil {
		return nil, fmt.Errorf("open web assets (build with scripts/build-ui.sh web): %w", err)
	}
	info, statErr := index.Stat()
	closeErr := index.Close()
	if statErr != nil {
		return nil, statErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("web index.html must be a regular file")
	}
	mux := s.handler(token, true)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		file, err := os.OpenInRoot(directory, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, path.Base(name), info.ModTime(), file)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	}), nil
}
