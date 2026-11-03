package lens

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func parseMessage(v Event, requestMethod string) (*Message, string, string, string, int, error) {
	m := &Message{Headers: make(http.Header), Body: append([]byte{}, v.Payload...), Raw: append([]byte{}, v.Payload...), Timestamp: v.Timestamp, Sequence: v.Sequence}
	proto := v.Protocol
	data := v.Payload
	first := string(bytes.SplitN(data, []byte("\n"), 2)[0])
	response := strings.HasPrefix(first, "HTTP/")
	looksHTTP := response || strings.Contains(first, " HTTP/")
	if !looksHTTP {
		return m, proto, "", "", 0, nil
	}
	if response {
		r, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), &http.Request{Method: requestMethod})
		if err != nil {
			return nil, "", "", "", 0, fmt.Errorf("parse HTTP response: %w", err)
		}
		body, err := io.ReadAll(r.Body)
		err = errors.Join(err, r.Body.Close())
		if err != nil {
			return nil, "", "", "", 0, err
		}
		m.Headers = r.Header
		m.Body = body
		if proto == "" {
			proto = r.Proto
		}
		return m, proto, "", "", r.StatusCode, nil
	}
	reader := bufio.NewReader(bytes.NewReader(data))
	r, err := http.ReadRequest(reader)
	if err != nil {
		return nil, "", "", "", 0, fmt.Errorf("parse HTTP request: %w", err)
	}
	body, err := io.ReadAll(r.Body)
	err = errors.Join(err, r.Body.Close())
	if err != nil {
		return nil, "", "", "", 0, err
	}
	m.Headers = r.Header
	m.Body = body
	// Reconstructed HTTP/2 messages may omit Content-Length; their event boundary frames the body.
	if r.ProtoMajor >= 2 && r.Body == http.NoBody {
		m.Body, err = io.ReadAll(reader)
		if err != nil {
			return nil, "", "", "", 0, err
		}
	}
	if r.Host != "" {
		m.Headers.Set("Host", r.Host)
	}
	uri := r.URL.String()
	// Upstream reconstructed HTTP/2 events can omit authority; retain their relative target.
	if !r.URL.IsAbs() && r.Host != "" {
		uri = "https://" + r.Host + r.URL.RequestURI()
	}
	if proto == "" {
		proto = r.Proto
	}
	return m, proto, r.Method, uri, 0, nil
}

func (e *Engine) ingest(v Event) error {
	if strings.TrimSpace(v.DeviceID) == "" || strings.TrimSpace(v.ConnectionID) == "" {
		return errors.New("device_id and connection_id are required")
	}
	if v.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	if v.Direction == "" {
		if bytes.HasPrefix(v.Payload, []byte("HTTP/")) {
			v.Direction = "response"
		} else if bytes.Contains(bytes.SplitN(v.Payload, []byte("\n"), 2)[0], []byte(" HTTP/")) {
			v.Direction = "request"
		}
	}
	if v.Direction != "request" && v.Direction != "response" {
		return errors.New("direction must be request or response")
	}
	requestMethod := ""
	key := v.DeviceID + "\x00" + v.ConnectionID
	missingRequest := v.Direction == "response" && v.StreamID == "" && len(e.state.Dropped[key]) > 0
	if missingRequest && e.state.Dropped[key][0] {
		requestMethod = "HEAD"
	}
	if v.Direction == "response" && !missingRequest {
		for _, t := range e.state.Requests {
			if t.DeviceID == v.DeviceID && t.ConnectionID == v.ConnectionID && t.StreamID == v.StreamID && t.Response == nil && t.Request != nil {
				requestMethod = t.Method
				break
			}
		}
	}
	m, proto, method, uri, status, err := parseMessage(v, requestMethod)
	if err != nil {
		return err
	}
	if status > 0 && v.Direction != "response" || method != "" && v.Direction != "request" {
		return errors.New("HTTP message conflicts with direction")
	}
	if proto == "" {
		proto = "raw"
	}
	if strings.HasPrefix(strings.ToLower(proto), "http/2") && v.StreamID == "" {
		return errors.New("HTTP/2 requires stream_id for multiplexed pairing")
	}
	var t *Transaction
	// ponytail: bounded 1000-row scan; add a pending-key index only if capture throughput requires it.
	for i := range e.state.Requests {
		x := &e.state.Requests[i]
		if x.DeviceID != v.DeviceID || x.ConnectionID != v.ConnectionID || x.StreamID != v.StreamID {
			continue
		}
		if missingRequest {
			if x.Error == "request evicted by history limit" && x.Response == nil {
				t = x
				break
			}
			continue
		}
		if x.Error != "" {
			continue
		}
		if v.Direction == "request" && x.Response != nil && v.StreamID == "" && v.Timestamp.After(x.Response.Timestamp) {
			continue
		}
		if v.Direction == "request" && x.Request == nil || v.Direction == "response" && x.Response == nil {
			t = x
			break
		}
	}
	if t == nil {
		e.state.NextID++
		e.state.Requests = append(e.state.Requests, Transaction{ID: fmt.Sprintf("%x", e.state.NextID), DeviceID: v.DeviceID, ConnectionID: v.ConnectionID, StreamID: v.StreamID, Protocol: proto, StartedAt: v.Timestamp, PID: v.PID, Process: v.Process, Src: v.Src, Dst: v.Dst})
		t = &e.state.Requests[len(e.state.Requests)-1]
		if missingRequest {
			t.Error = "request evicted by history limit"
		}
	}
	if v.Direction == "request" {
		t.Request = m
		t.Method = method
		t.URL = uri
		t.StartedAt = v.Timestamp
		t.Protocol = proto
		if t.PID == 0 {
			t.PID = v.PID
		}
		if t.Process == "" {
			t.Process = v.Process
		}
	}
	if v.Direction == "response" {
		if status >= 100 && status < 200 && status != 101 {
			t.Interim = append(t.Interim, m)
		} else {
			t.Response = m
			t.Status = status
			if missingRequest {
				e.state.Dropped[key] = e.state.Dropped[key][1:]
				if len(e.state.Dropped[key]) == 0 {
					delete(e.state.Dropped, key)
				}
			}
		}
	}
	if t.Request != nil && t.Response != nil {
		t.DurationMS = float64(t.Response.Timestamp.Sub(t.Request.Timestamp)) / 1e6
	}
	return e.pruneHistory()
}

func (e *Engine) pruneHistory() error {
	for len(e.state.Requests) > HistoryLimit {
		old := e.state.Requests[0]
		if old.StreamID == "" && old.Request != nil && old.Response == nil && old.Error == "" {
			if e.state.Dropped == nil {
				e.state.Dropped = map[string][]bool{}
			}
			oldKey := old.DeviceID + "\x00" + old.ConnectionID
			e.state.Dropped[oldKey] = append(e.state.Dropped[oldKey], old.Method == "HEAD")
			// ponytail: bound abandoned FIFO slots to 1000 per connection across 1000 connections; reset capture when exhausted.
			if len(e.state.Dropped) > HistoryLimit || len(e.state.Dropped[oldKey]) > HistoryLimit {
				return errors.New("evicted pending FIFO tracking exhausted (1000 slots per connection, 1000 connections); clear history before starting a new capture")
			}
		}
		e.state.Requests = e.state.Requests[1:]
	}
	return nil
}
