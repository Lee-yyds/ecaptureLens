package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIAuthAndErrors(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := s.Handler("secret")
	for _, tc := range []struct {
		body, token string
		want        int
	}{{`{"method":"system.info"}`, "", 401}, {`{"method":"system.info"}`, "secret", 200}, {`{"method":"missing"}`, "secret", 400}, {`{`, "secret", 400}, {`{"method":"system.info"} {}`, "secret", 400}} {
		r := httptest.NewRequest("POST", "/api", bytes.NewBufferString(tc.body))
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s got %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
}
func TestCaptureUpstreamOriginAndIngest(t *testing.T) {
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") != "" }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		close(connected)
		raw := "GET /origin HTTP/2.0\r\nHost: example.org\r\n\r\n"
		if err = conn.WriteMessage(websocket.BinaryMessage, eventFrame(raw, 1)); err != nil {
			return
		}
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, _ := json.Marshal(map[string]string{"id": "origin", "name": "origin", "endpoint": "ws" + strings.TrimPrefix(server.URL, "http")})
	if _, err = s.Call(context.Background(), "devices.put", p); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Call(context.Background(), "capture.start", json.RawMessage(`{"device_id":"origin"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatalf("missing handshake origin; status: %+v", s.status("origin"))
	}
	deadline := time.After(time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("message not ingested")
		case <-tick.C:
			v, err := s.Call(context.Background(), "requests.list", nil)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(v)
			var result struct{ Total int }
			if err = json.Unmarshal(b, &result); err != nil {
				t.Fatal(err)
			}
			if result.Total == 1 {
				return
			}
		}
	}
}
func TestCaptureReportsCommandFailure(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, err = s.Call(ctx, "devices.put", json.RawMessage(`{"id":"broken","name":"broken","start_command":"exit 7"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Call(ctx, "capture.start", json.RawMessage(`{"device_id":"broken"}`))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("command failure not surfaced")
		case <-tick.C:
			v, err := s.Call(ctx, "capture.status", nil)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(v)
			var statuses []CaptureStatus
			if err = json.Unmarshal(b, &statuses); err != nil {
				t.Fatal(err)
			}
			if len(statuses) == 1 && statuses[0].State == "error" {
				if statuses[0].Error == "" {
					t.Fatal("empty command error")
				}
				return
			}
		}
	}
}
func TestCaptureStartupCommandErrorWinsOverPendingDial(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Call(context.Background(), "devices.put", json.RawMessage(`{"id":"failed-start","endpoint":"ws://127.0.0.1:1/","start_command":"echo start-failed >&2; exit 7"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Call(context.Background(), "capture.start", json.RawMessage(`{"device_id":"failed-start"}`))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatalf("startup command failure hidden by pending dial: %+v", s.status("failed-start"))
		case <-tick.C:
			statuses := s.status("failed-start")
			if len(statuses) == 1 && statuses[0].State == "error" {
				if !strings.Contains(statuses[0].Error, "exit status 7") || !strings.Contains(statuses[0].Error, "start-failed") {
					t.Fatalf("wrong cause: %+v", statuses)
				}
				return
			}
		}
	}
}
