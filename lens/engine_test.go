package lens

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func call(t *testing.T, e *Engine, method string, params any) any {
	t.Helper()
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Call(context.Background(), method, b)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return v
}

func event(device, conn, stream, direction, payload string) Event {
	return Event{DeviceID: device, ConnectionID: conn, StreamID: stream, Direction: direction, Timestamp: time.Now().UTC(), Payload: []byte(payload)}
}

func TestPairingDeviceIsolationAndHTTP2(t *testing.T) {
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	events := []Event{
		event("a", "c", "1", "request", "GET /one HTTP/2.0\r\nHost: api.test\r\n\r\n"),
		event("a", "c", "3", "request", "POST /three HTTP/2.0\r\nHost: api.test\r\nContent-Length: 2\r\n\r\n{}"),
		event("b", "c", "1", "request", "GET /other HTTP/2.0\r\nHost: api.test\r\n\r\n"),
		event("a", "c", "3", "response", "HTTP/2.0 201 Created\r\nContent-Length: 2\r\n\r\n{}"),
		event("a", "c", "1", "response", "HTTP/2.0 200 OK\r\nContent-Length: 2\r\n\r\n[]"),
	}
	call(t, e, "events.ingest", map[string]any{"events": events})
	list := call(t, e, "requests.list", map[string]any{}).(ListResult)
	if list.Total != 3 {
		t.Fatalf("got %d transactions", list.Total)
	}
	for _, x := range list.Items {
		if x.DeviceID == "b" && x.Response != nil {
			t.Fatal("cross-device response paired")
		}
		if x.DeviceID == "a" && x.StreamID == "3" && x.Status != 201 {
			t.Fatalf("bad stream pairing: %+v", x)
		}
		if x.DeviceID == "a" && x.StreamID == "1" && x.Status != 200 {
			t.Fatalf("bad stream pairing: %+v", x)
		}
	}
}

func TestHTTP1FIFOInterimAndResponseFirst(t *testing.T) {
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	events := []Event{
		event("a", "c", "", "request", "POST /one HTTP/1.1\r\nHost: a\r\nContent-Length: 0\r\n\r\n"),
		event("a", "c", "", "request", "GET /two HTTP/1.1\r\nHost: a\r\n\r\n"),
		event("a", "c", "", "response", "HTTP/1.1 100 Continue\r\n\r\n"),
		event("a", "c", "", "response", "HTTP/1.1 201 Created\r\nContent-Length: 0\r\n\r\n"),
		event("a", "c", "", "response", "HTTP/1.1 202 Accepted\r\nContent-Length: 0\r\n\r\n"),
		event("a", "x", "9", "response", "HTTP/2.0 204 No Content\r\n\r\n"),
		event("a", "x", "9", "request", "GET /late HTTP/2.0\r\nHost: a\r\n\r\n"),
	}
	call(t, e, "events.ingest", map[string]any{"events": events})
	list := call(t, e, "requests.list", map[string]any{}).(ListResult)
	if list.Total != 3 {
		t.Fatal(list.Total)
	}
	for _, x := range list.Items {
		if x.URL == "https://a/one" && x.Status != 201 {
			t.Fatal(x)
		}
		if x.URL == "https://a/two" && x.Status != 202 {
			t.Fatal(x)
		}
		if x.StreamID == "9" && (x.Request == nil || x.Response == nil) {
			t.Fatal(x)
		}
	}
}

func TestRetentionRestartAndLock(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Open(dir); err == nil {
		second.Close()
		t.Fatal("second writer accepted")
	}
	var events []Event
	for i := 0; i < 1005; i++ {
		events = append(events, event("a", fmt.Sprint(i), "", "request", fmt.Sprintf("GET /%d HTTP/1.1\r\nHost: a\r\n\r\n", i)))
	}
	call(t, e, "events.ingest", map[string]any{"events": events})
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	x := call(t, e, "requests.list", map[string]any{}).(ListResult)
	if x.Total != 1000 {
		t.Fatal(x.Total)
	}
	if x.Items[0].URL != "https://a/1004" {
		t.Fatal(x.Items[0].URL)
	}
}

func TestBoundaryFailureDoesNotMutate(t *testing.T) {
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	good := event("a", "c", "", "request", "GET / HTTP/1.1\r\nHost: a\r\n\r\n")
	bad := event("", "c", "", "request", "bad")
	b, _ := json.Marshal(map[string]any{"events": []Event{good, bad}})
	if _, err = e.Call(context.Background(), "events.ingest", b); err == nil {
		t.Fatal("invalid device accepted")
	}
	if call(t, e, "requests.list", map[string]any{}).(ListResult).Total != 0 {
		t.Fatal("partial mutation")
	}
	if _, err = e.Call(context.Background(), "history.import", json.RawMessage(`{"format":"json","content":"[invalid]"}`)); err == nil {
		t.Fatal("invalid import accepted")
	}
	if _, err = e.Call(context.Background(), "requests.list", json.RawMessage(`{"offset":-1}`)); err == nil {
		t.Fatal("negative offset accepted")
	}
}

func TestCorruptHistoryIsVisible(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if e, err := Open(dir); err == nil {
		e.Close()
		t.Fatal("corrupt state accepted")
	}
}

func TestEvictedPendingRequestDoesNotShiftFIFO(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]Event, 1001)
	for i := range events {
		method := "GET"
		if i == 0 {
			method = "HEAD"
		}
		events[i] = event("a", "pipeline", "", "request", fmt.Sprintf("%s /%d HTTP/1.1\r\nHost: a\r\n\r\n", method, i))
	}
	call(t, e, "events.ingest", map[string]any{"events": events})
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	call(t, e, "events.ingest", map[string]any{"events": []Event{event("a", "pipeline", "", "response", "HTTP/1.1 200 OK\r\nContent-Length: 500\r\n\r\n")}})
	items := call(t, e, "requests.list", map[string]any{}).(ListResult).Items
	if items[0].Request != nil || items[0].Response == nil || items[0].Error == "" {
		t.Fatalf("evicted request incorrectly paired: %+v", items[0])
	}
	for _, v := range items {
		if v.Request != nil && v.Response != nil {
			t.Fatal("FIFO shifted to retained request")
		}
	}
}

func TestReturnedTransactionsDoNotAliasCoreState(t *testing.T) {
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	call(t, e, "events.ingest", map[string]any{"events": []Event{event("a", "c", "", "request", "POST / HTTP/1.1\r\nHost: a\r\nContent-Length: 2\r\n\r\n{}")}})
	list := call(t, e, "requests.list", map[string]any{}).(ListResult)
	list.Items[0].Request.Body[0] = '!'
	list.Items[0].Request.Headers.Set("Host", "changed")
	got := call(t, e, "requests.get", map[string]any{"id": list.Items[0].ID}).(Transaction)
	if string(got.Request.Body) != "{}" || got.Request.Headers.Get("Host") != "a" {
		t.Fatal("caller mutated core state")
	}
}
