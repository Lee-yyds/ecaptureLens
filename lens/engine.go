package lens

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gofrs/flock"
)

var Methods = []string{"system.info", "events.ingest", "requests.list", "requests.get", "requests.clear", "sessions.list", "sessions.timeline", "body.view", "history.export", "history.import", "devices.list", "devices.get", "devices.put", "devices.delete", "decoders.list", "decoders.put", "decoders.delete"}

type Engine struct {
	mu     sync.Mutex
	dir    string
	lock   *flock.Flock
	state  state
	closed bool
}

func Open(dir string) (*Engine, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(dir, "state.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("data directory is in use; connect to the running service instead")
	}
	e := &Engine{dir: dir, lock: lock, state: state{Requests: []Transaction{}, Devices: []Device{}, Decoders: []Decoder{}, Dropped: map[string][]bool{}}}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err == nil {
		if err = json.Unmarshal(b, &e.state); err == nil {
			err = validateState(e.state)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("read history: %w", err), lock.Unlock())
	}
	return e, nil
}

func validateState(s state) error {
	if len(s.Requests) > HistoryLimit {
		return fmt.Errorf("history exceeds %d transactions", HistoryLimit)
	}
	seen := make(map[string]bool, len(s.Requests))
	for _, t := range s.Requests {
		if t.ID == "" || t.DeviceID == "" || t.ConnectionID == "" || t.StartedAt.IsZero() {
			return errors.New("history transaction requires id, device_id, connection_id and started_at")
		}
		if seen[t.ID] {
			return fmt.Errorf("duplicate transaction id %s", t.ID)
		}
		n, err := strconv.ParseUint(t.ID, 16, 64)
		if err != nil || n == 0 || n > s.NextID {
			return errors.New("history transaction ID is inconsistent with next_id")
		}
		seen[t.ID] = true
	}
	if len(s.Dropped) > HistoryLimit {
		return errors.New("history has too many evicted pending connections")
	}
	for _, slots := range s.Dropped {
		if len(slots) > HistoryLimit {
			return errors.New("history has too many evicted pending slots")
		}
	}
	return nil
}

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	return e.lock.Unlock()
}

func (e *Engine) save(s state) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(e.dir, "state-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		return errors.Join(err, f.Close())
	}
	if _, err = f.Write(b); err != nil {
		return errors.Join(err, f.Close())
	}
	if err = f.Sync(); err != nil {
		return errors.Join(err, f.Close())
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(e.dir, "state.json"))
}

func params(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	return json.Unmarshal(raw, v)
}

func (e *Engine) Call(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil, errors.New("engine is closed")
	}
	if err := ctx.Err(); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if method == "body.view" {
		snapshot := e.state
		e.mu.Unlock()
		return viewBody(ctx, raw, snapshot)
	}
	defer e.mu.Unlock()
	// ponytail: history is bounded at 1000; clone before each mutation for atomic rollback.
	mutating := method == "events.ingest" || method == "requests.clear" || method == "history.import" || strings.HasSuffix(method, ".put") || strings.HasSuffix(method, ".delete")
	if mutating {
		b, err := json.Marshal(e.state)
		if err != nil {
			return nil, err
		}
		var next state
		if err = json.Unmarshal(b, &next); err != nil {
			return nil, err
		}
		old := e.state
		e.state = next
		result, err := e.call(ctx, method, raw)
		if err == nil {
			err = e.save(e.state)
		}
		if err != nil {
			e.state = old
			return nil, err
		}
		return result, nil
	}
	return e.call(ctx, method, raw)
}

func (e *Engine) call(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	switch method {
	case "system.info":
		return map[string]any{"version": "0.1.0", "history_limit": HistoryLimit, "methods": append([]string{}, Methods...)}, nil
	case "events.ingest":
		var p struct {
			Events []Event `json:"events"`
		}
		if err := params(raw, &p); err != nil {
			return nil, err
		}
		for i, event := range p.Events {
			if err := e.ingest(event); err != nil {
				return nil, fmt.Errorf("event %d: %w", i, err)
			}
		}
		return map[string]int{"ingested": len(p.Events)}, nil
	case "requests.list":
		return e.list(raw)
	case "requests.get":
		var p struct {
			ID string `json:"id"`
		}
		if err := params(raw, &p); err != nil {
			return nil, err
		}
		t, err := e.find(p.ID)
		if err != nil {
			return nil, err
		}
		return copyTransaction(*t), nil
	case "requests.clear":
		e.state.Requests = []Transaction{}
		e.state.Dropped = map[string][]bool{}
		return map[string]bool{"cleared": true}, nil
	case "sessions.list", "sessions.timeline":
		return e.sessions(method, raw)
	case "history.export":
		return e.export(raw)
	case "history.import":
		return e.importHistory(raw)
	case "devices.list":
		return append([]Device{}, e.state.Devices...), nil
	case "devices.get", "devices.put", "devices.delete":
		return e.devices(method, raw)
	case "decoders.list":
		return append([]Decoder{}, e.state.Decoders...), nil
	case "decoders.put", "decoders.delete":
		return e.decoders(method, raw)
	default:
		return nil, fmt.Errorf("unknown method %q", method)
	}
}

func (e *Engine) find(id string) (*Transaction, error) {
	for i := range e.state.Requests {
		if e.state.Requests[i].ID == id {
			return &e.state.Requests[i], nil
		}
	}
	return nil, fmt.Errorf("request %q not found", id)
}

func (e *Engine) list(raw json.RawMessage) (ListResult, error) {
	var p struct {
		Query        string `json:"query"`
		DeviceID     string `json:"device_id"`
		ConnectionID string `json:"connection_id"`
		Limit        int    `json:"limit"`
		Offset       int    `json:"offset"`
	}
	if err := params(raw, &p); err != nil {
		return ListResult{}, err
	}
	if p.Limit < 0 || p.Offset < 0 {
		return ListResult{}, errors.New("limit and offset must be nonnegative")
	}
	out := ListResult{Items: []Transaction{}}
	for i := len(e.state.Requests) - 1; i >= 0; i-- {
		t := e.state.Requests[i]
		if p.DeviceID != "" && p.DeviceID != t.DeviceID || p.ConnectionID != "" && p.ConnectionID != t.ConnectionID {
			continue
		}
		if p.Query != "" {
			b, err := json.Marshal(t)
			if err != nil {
				return ListResult{}, err
			}
			hay := strings.ToLower(string(b))
			if t.Request != nil {
				hay += strings.ToLower(string(t.Request.Body))
			}
			if t.Response != nil {
				hay += strings.ToLower(string(t.Response.Body))
			}
			if !strings.Contains(hay, strings.ToLower(p.Query)) {
				continue
			}
		}
		out.Total++
		if out.Total <= p.Offset {
			continue
		}
		if p.Limit == 0 || len(out.Items) < p.Limit {
			out.Items = append(out.Items, copyTransaction(t))
		}
	}
	return out, nil
}

func (e *Engine) sessions(method string, raw json.RawMessage) (any, error) {
	var p struct {
		DeviceID     string `json:"device_id"`
		ConnectionID string `json:"connection_id"`
	}
	if err := params(raw, &p); err != nil {
		return nil, err
	}
	if method == "sessions.timeline" {
		if p.ConnectionID == "" {
			return nil, errors.New("connection_id is required")
		}
		out := []Transaction{}
		for _, t := range e.state.Requests {
			if t.ConnectionID == p.ConnectionID && (p.DeviceID == "" || t.DeviceID == p.DeviceID) {
				out = append(out, copyTransaction(t))
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
		return out, nil
	}
	byKey := map[string]*Session{}
	keys := []string{}
	for _, t := range e.state.Requests {
		if p.DeviceID != "" && t.DeviceID != p.DeviceID {
			continue
		}
		key := t.DeviceID + "\x00" + t.ConnectionID
		s := byKey[key]
		if s == nil {
			s = &Session{DeviceID: t.DeviceID, ConnectionID: t.ConnectionID, StartedAt: t.StartedAt, EndedAt: t.StartedAt}
			byKey[key] = s
			keys = append(keys, key)
		}
		s.Count++
		if t.StartedAt.Before(s.StartedAt) {
			s.StartedAt = t.StartedAt
		}
		end := t.StartedAt
		if t.Response != nil {
			end = t.Response.Timestamp
		}
		if end.After(s.EndedAt) {
			s.EndedAt = end
		}
	}
	out := make([]Session, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byKey[key])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

func (e *Engine) devices(method string, raw json.RawMessage) (any, error) {
	var d Device
	if err := params(raw, &d); err != nil {
		return nil, err
	}
	if strings.TrimSpace(d.ID) == "" {
		return nil, errors.New("device id is required")
	}
	index := -1
	for i := range e.state.Devices {
		if e.state.Devices[i].ID == d.ID {
			index = i
			break
		}
	}
	switch method {
	case "devices.get":
		if index < 0 {
			return nil, fmt.Errorf("device %q not found", d.ID)
		}
		return e.state.Devices[index], nil
	case "devices.put":
		if d.Name == "" {
			d.Name = d.ID
		}
		if index < 0 {
			e.state.Devices = append(e.state.Devices, d)
		} else {
			e.state.Devices[index] = d
		}
		return d, nil
	default:
		if index < 0 {
			return nil, fmt.Errorf("device %q not found", d.ID)
		}
		e.state.Devices = append(e.state.Devices[:index], e.state.Devices[index+1:]...)
		return map[string]bool{"deleted": true}, nil
	}
}

func (e *Engine) decoders(method string, raw json.RawMessage) (any, error) {
	var d Decoder
	if err := params(raw, &d); err != nil {
		return nil, err
	}
	if d.Name == "" {
		return nil, errors.New("decoder name is required")
	}
	for _, name := range []string{"json", "text", "hex", "protobuf", "grpc"} {
		if name == d.Name {
			return nil, errors.New("built-in decoder names are reserved")
		}
	}
	index := -1
	for i := range e.state.Decoders {
		if e.state.Decoders[i].Name == d.Name {
			index = i
			break
		}
	}
	if method == "decoders.put" {
		if strings.TrimSpace(d.Command) == "" {
			return nil, errors.New("decoder command is required")
		}
		if index < 0 {
			e.state.Decoders = append(e.state.Decoders, d)
		} else {
			e.state.Decoders[index] = d
		}
		return d, nil
	}
	if index < 0 {
		return nil, fmt.Errorf("decoder %q not found", d.Name)
	}
	e.state.Decoders = append(e.state.Decoders[:index], e.state.Decoders[index+1:]...)
	return map[string]bool{"deleted": true}, nil
}
