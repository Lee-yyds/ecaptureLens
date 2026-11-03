package lens

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type harContent struct {
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
	Encoding string `json:"encoding,omitempty"`
}
type harRequest struct {
	Method      string      `json:"method"`
	URL         string      `json:"url"`
	HTTPVersion string      `json:"httpVersion"`
	Headers     []harHeader `json:"headers"`
	QueryString []harHeader `json:"queryString"`
	Cookies     []any       `json:"cookies"`
	HeadersSize int         `json:"headersSize"`
	BodySize    int         `json:"bodySize"`
	PostData    *harContent `json:"postData,omitempty"`
}
type harResponse struct {
	Status      int         `json:"status"`
	StatusText  string      `json:"statusText"`
	HTTPVersion string      `json:"httpVersion"`
	Headers     []harHeader `json:"headers"`
	Cookies     []any       `json:"cookies"`
	Content     harContent  `json:"content"`
	RedirectURL string      `json:"redirectURL"`
	HeadersSize int         `json:"headersSize"`
	BodySize    int         `json:"bodySize"`
}
type harEntry struct {
	StartedDateTime time.Time          `json:"startedDateTime"`
	Time            float64            `json:"time"`
	Request         harRequest         `json:"request"`
	Response        harResponse        `json:"response"`
	Cache           map[string]any     `json:"cache"`
	Timings         map[string]float64 `json:"timings"`
	Lens            *Transaction       `json:"_ecapturelens,omitempty"`
}
type harLog struct {
	Version string            `json:"version"`
	Creator map[string]string `json:"creator"`
	Entries []harEntry        `json:"entries"`
}
type har struct {
	Log harLog `json:"log"`
}

func headersToHAR(h http.Header) []harHeader {
	out := []harHeader{}
	for k, vs := range h {
		for _, v := range vs {
			out = append(out, harHeader{k, v})
		}
	}
	return out
}
func headersFromHAR(h []harHeader) http.Header {
	out := make(http.Header)
	for _, v := range h {
		out.Add(v.Name, v.Value)
	}
	return out
}

func (e *Engine) export(raw json.RawMessage) (any, error) {
	var p struct {
		Format string   `json:"format"`
		IDs    []string `json:"ids"`
	}
	if err := params(raw, &p); err != nil {
		return nil, err
	}
	items := []Transaction{}
	wanted := map[string]bool{}
	for _, id := range p.IDs {
		wanted[id] = true
	}
	for _, t := range e.state.Requests {
		if len(p.IDs) == 0 || wanted[t.ID] {
			items = append(items, t)
			delete(wanted, t.ID)
		}
	}
	if len(wanted) > 0 {
		return nil, errors.New("one or more requested IDs do not exist")
	}
	var data any
	mime := "application/json"
	switch p.Format {
	case "json":
		data = items
	case "har":
		h := har{Log: harLog{Version: "1.2", Creator: map[string]string{"name": "eCaptureLens", "version": "0.1.0"}, Entries: []harEntry{}}}
		for _, t := range items {
			entry := harEntry{StartedDateTime: t.StartedAt, Time: t.DurationMS, Lens: &t, Cache: map[string]any{}, Timings: map[string]float64{"send": 0, "wait": t.DurationMS, "receive": 0}, Request: harRequest{Method: t.Method, URL: t.URL, HTTPVersion: t.Protocol, Headers: []harHeader{}, QueryString: []harHeader{}, Cookies: []any{}, HeadersSize: -1}, Response: harResponse{Status: t.Status, StatusText: http.StatusText(t.Status), HTTPVersion: t.Protocol, Headers: []harHeader{}, Cookies: []any{}, HeadersSize: -1}}
			if u, err := url.Parse(t.URL); err == nil {
				for k, vs := range u.Query() {
					for _, v := range vs {
						entry.Request.QueryString = append(entry.Request.QueryString, harHeader{k, v})
					}
				}
			}
			if t.Request != nil {
				entry.Request.Headers = headersToHAR(t.Request.Headers)
				entry.Request.BodySize = len(t.Request.Body)
				if len(t.Request.Body) > 0 {
					entry.Request.PostData = &harContent{Size: len(t.Request.Body), MimeType: t.Request.Headers.Get("Content-Type"), Text: base64.StdEncoding.EncodeToString(t.Request.Body), Encoding: "base64"}
					if utf8.Valid(t.Request.Body) {
						entry.Request.PostData.Text = string(t.Request.Body)
						entry.Request.PostData.Encoding = ""
					}
				}
			}
			if t.Response != nil {
				entry.Response.Headers = headersToHAR(t.Response.Headers)
				entry.Response.BodySize = len(t.Response.Body)
				entry.Response.Content = harContent{Size: len(t.Response.Body), MimeType: t.Response.Headers.Get("Content-Type"), Text: base64.StdEncoding.EncodeToString(t.Response.Body), Encoding: "base64"}
				entry.Response.RedirectURL = t.Response.Headers.Get("Location")
			}
			h.Log.Entries = append(h.Log.Entries, entry)
		}
		data = h
	default:
		return nil, errors.New("format must be json or har")
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]string{"content": string(b), "mime": mime}, nil
}

func decodeHARContent(c harContent) ([]byte, error) {
	if c.Encoding == "base64" {
		return base64.StdEncoding.DecodeString(c.Text)
	}
	if c.Encoding != "" {
		return nil, fmt.Errorf("unsupported HAR content encoding %q", c.Encoding)
	}
	return []byte(c.Text), nil
}

func (e *Engine) importHistory(raw json.RawMessage) (any, error) {
	var p struct {
		Format  string `json:"format"`
		Content string `json:"content"`
	}
	if err := params(raw, &p); err != nil {
		return nil, err
	}
	var items []Transaction
	switch p.Format {
	case "json":
		if err := json.Unmarshal([]byte(p.Content), &items); err != nil {
			return nil, err
		}
	case "har":
		var h har
		if err := json.Unmarshal([]byte(p.Content), &h); err != nil {
			return nil, err
		}
		if h.Log.Version == "" {
			return nil, errors.New("missing HAR log.version")
		}
		for i, v := range h.Log.Entries {
			if v.Lens != nil {
				items = append(items, *v.Lens)
				continue
			}
			if v.StartedDateTime.IsZero() {
				return nil, fmt.Errorf("HAR entry %d has no startedDateTime", i)
			}
			if v.Time < 0 {
				return nil, fmt.Errorf("HAR entry %d has negative time", i)
			}
			if _, err := url.ParseRequestURI(v.Request.URL); err != nil {
				return nil, fmt.Errorf("HAR entry %d URL: %w", i, err)
			}
			t := Transaction{DeviceID: "imported", ConnectionID: fmt.Sprintf("har-%d-%d", e.state.NextID, i), Method: v.Request.Method, URL: v.Request.URL, Protocol: v.Request.HTTPVersion, Status: v.Response.Status, StartedAt: v.StartedDateTime, DurationMS: v.Time}
			t.Request = &Message{Headers: headersFromHAR(v.Request.Headers), Body: []byte{}, Raw: []byte{}, Timestamp: v.StartedDateTime}
			if v.Request.PostData != nil {
				b, err := decodeHARContent(*v.Request.PostData)
				if err != nil {
					return nil, err
				}
				t.Request.Body = b
			}
			b, err := decodeHARContent(v.Response.Content)
			if err != nil {
				return nil, err
			}
			t.Response = &Message{Headers: headersFromHAR(v.Response.Headers), Body: b, Raw: []byte{}, Timestamp: v.StartedDateTime.Add(time.Duration(v.Time * float64(time.Millisecond)))}
			items = append(items, t)
		}
	default:
		return nil, errors.New("format must be json or har")
	}
	for i := range items {
		t := &items[i]
		if strings.TrimSpace(t.DeviceID) == "" || strings.TrimSpace(t.ConnectionID) == "" || t.StartedAt.IsZero() {
			return nil, fmt.Errorf("transaction %d requires device_id, connection_id and started_at", i)
		}
		if t.Request == nil && t.Response == nil {
			return nil, fmt.Errorf("transaction %d has neither request nor response", i)
		}
		e.state.NextID++
		t.ID = fmt.Sprintf("%x", e.state.NextID)
	}
	e.state.Requests = append(e.state.Requests, items...)
	if err := e.pruneHistory(); err != nil {
		return nil, err
	}
	return map[string]int{"imported": len(items), "retained": len(e.state.Requests)}, nil
}
