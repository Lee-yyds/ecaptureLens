// Package gateway is the common product entry point for CLI, HTTP and Flutter.
package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"ecapturelens/lens"
)

type Service struct {
	core     *lens.Engine
	mu       sync.Mutex
	captures map[string]*capture
	closed   bool
}

func Open(dir string) (*Service, error) {
	core, err := lens.Open(dir)
	if err != nil {
		return nil, err
	}
	return &Service{core: core, captures: make(map[string]*capture)}, nil
}

func (s *Service) Call(ctx context.Context, method string, params json.RawMessage) (any, error) {
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	switch method {
	case "api.describe":
		return map[string]any{"version": "0.1.0", "transport": "POST /api {method,params}; Authorization: Bearer TOKEN", "methods": []string{"system.info", "events.ingest", "requests.list", "requests.get", "requests.clear", "sessions.list", "sessions.timeline", "body.view", "history.export", "history.import", "devices.list", "devices.get", "devices.put", "devices.delete", "devices.discover", "decoders.list", "decoders.put", "decoders.delete", "capture.start", "capture.stop", "capture.status", "api.describe"}}, nil
	case "devices.discover":
		return discoverDevices(ctx)
	case "capture.start", "capture.stop", "capture.status":
		var p struct {
			DeviceID string `json:"device_id"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if method == "capture.status" {
			return s.status(p.DeviceID), nil
		}
		if p.DeviceID == "" {
			return nil, fmt.Errorf("device_id is required")
		}
		if method == "capture.start" {
			return s.start(ctx, p.DeviceID)
		}
		return s.stop(ctx, p.DeviceID)
	default:
		return s.core.Call(ctx, method, params)
	}
}

func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	ids := make([]string, 0, len(s.captures))
	for id := range s.captures {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	var result error
	for _, id := range ids {
		_, err := s.stop(context.Background(), id)
		result = errors.Join(result, err)
	}
	return errors.Join(result, s.core.Close())
}

func (s *Service) Handler(token string) http.Handler {
	return s.handler(token, false)
}

func (s *Service) handler(token string, browser bool) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": "0.1.0"})
	})
	mux.HandleFunc("POST /api", func(w http.ResponseWriter, r *http.Request) {
		if token != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			writeJSON(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if !browser || origin != scheme+"://"+r.Host {
				writeJSON(w, 403, map[string]string{"error": "browser origin is not allowed"})
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
		var request struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&request); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			writeJSON(w, 400, map[string]string{"error": "expected exactly one JSON request"})
			return
		}
		if request.Method == "" {
			writeJSON(w, 400, map[string]string{"error": "method is required"})
			return
		}
		result, err := s.Call(r.Context(), request.Method, request.Params)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"result": result})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}
