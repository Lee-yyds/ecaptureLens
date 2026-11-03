package lens

import (
	"bytes"
	"net/http"
	"time"
)

const HistoryLimit = 1000

type Event struct {
	DeviceID     string    `json:"device_id"`
	ConnectionID string    `json:"connection_id"`
	StreamID     string    `json:"stream_id"`
	Direction    string    `json:"direction"`
	Timestamp    time.Time `json:"timestamp"`
	Protocol     string    `json:"protocol,omitempty"`
	Src          string    `json:"src,omitempty"`
	Dst          string    `json:"dst,omitempty"`
	Payload      []byte    `json:"payload"`
	PID          int       `json:"pid,omitempty"`
	Process      string    `json:"process,omitempty"`
	Sequence     uint64    `json:"sequence,omitempty"`
}

type Message struct {
	Headers   http.Header `json:"headers"`
	Body      []byte      `json:"body"`
	Raw       []byte      `json:"raw"`
	Timestamp time.Time   `json:"timestamp"`
	Sequence  uint64      `json:"sequence,omitempty"`
}

type Transaction struct {
	ID           string     `json:"id"`
	DeviceID     string     `json:"device_id"`
	ConnectionID string     `json:"connection_id"`
	StreamID     string     `json:"stream_id"`
	Protocol     string     `json:"protocol"`
	Method       string     `json:"method"`
	URL          string     `json:"url"`
	Status       int        `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	DurationMS   float64    `json:"duration_ms"`
	Request      *Message   `json:"request"`
	Response     *Message   `json:"response"`
	Interim      []*Message `json:"interim,omitempty"`
	PID          int        `json:"pid,omitempty"`
	Process      string     `json:"process,omitempty"`
	Src          string     `json:"src,omitempty"`
	Dst          string     `json:"dst,omitempty"`
	Error        string     `json:"error,omitempty"`
}

type Device struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	StartCommand string `json:"start_command"`
	StopCommand  string `json:"stop_command"`
}

type Decoder struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type ListResult struct {
	Items []Transaction `json:"items"`
	Total int           `json:"total"`
}

type Session struct {
	DeviceID     string    `json:"device_id"`
	ConnectionID string    `json:"connection_id"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	Count        int       `json:"count"`
}

type state struct {
	Requests []Transaction     `json:"requests"`
	Devices  []Device          `json:"devices"`
	Decoders []Decoder         `json:"decoders"`
	NextID   uint64            `json:"next_id"`
	Dropped  map[string][]bool `json:"dropped"`
}

func copyMessage(m *Message) *Message {
	if m == nil {
		return nil
	}
	out := *m
	out.Headers = m.Headers.Clone()
	out.Body = bytes.Clone(m.Body)
	out.Raw = bytes.Clone(m.Raw)
	return &out
}

func copyTransaction(t Transaction) Transaction {
	t.Request = copyMessage(t.Request)
	t.Response = copyMessage(t.Response)
	t.Interim = append([]*Message(nil), t.Interim...)
	for i, m := range t.Interim {
		t.Interim[i] = copyMessage(m)
	}
	return t
}
