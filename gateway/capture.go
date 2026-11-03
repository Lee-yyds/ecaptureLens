package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

type CaptureStatus struct {
	DeviceID string `json:"device_id"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
	Logs     string `json:"logs,omitempty"`
}
type capture struct {
	status      CaptureStatus
	cancel      context.CancelFunc
	done        chan struct{}
	log         boundedLog
	stopCommand string
	stopOnce    sync.Once
	stopErr     error
}
type deviceConfig struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
	Start    string `json:"start_command"`
	Stop     string `json:"stop_command"`
}

func (s *Service) device(ctx context.Context, id string) (deviceConfig, error) {
	p, _ := json.Marshal(map[string]string{"id": id})
	v, err := s.core.Call(ctx, "devices.get", p)
	if err != nil {
		return deviceConfig{}, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return deviceConfig{}, err
	}
	var d deviceConfig
	err = json.Unmarshal(b, &d)
	return d, err
}
func (s *Service) status(id string) []CaptureStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CaptureStatus, 0, len(s.captures))
	for key, c := range s.captures {
		if id == "" || id == key {
			v := c.status
			v.Logs = c.log.String()
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })
	return out
}
func (s *Service) start(ctx context.Context, id string) (any, error) {
	d, err := s.device(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.Endpoint == "" && d.Start == "" {
		return nil, fmt.Errorf("configure an eCapture WebSocket endpoint or a start command for %s", id)
	}
	if d.Endpoint != "" {
		u, err := url.Parse(d.Endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "ws" && u.Scheme != "wss") {
			return nil, fmt.Errorf("endpoint must be ws:// or wss://")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("service closed")
	}
	if old, ok := s.captures[id]; ok {
		select {
		case <-old.done:
		default:
			return nil, fmt.Errorf("capture already active for %s", id)
		}
	}
	runCtx, cancel := context.WithCancel(context.Background())
	c := &capture{status: CaptureStatus{DeviceID: id, State: "starting"}, cancel: cancel, done: make(chan struct{}), stopCommand: d.Stop}
	s.captures[id] = c
	go s.runCapture(runCtx, c, d)
	return c.status, nil
}
func (s *Service) setState(c *capture, state string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.status.State = state
	if err != nil {
		c.status.Error = err.Error()
	}
}

func (s *Service) runCapture(ctx context.Context, c *capture, d deviceConfig) {
	defer close(c.done)
	runCtx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(nil)
	var cmd *exec.Cmd
	var processDone chan error
	if d.Start != "" {
		cmd = shellCommand(ctx, d.Start)
		cmd.Stdout = &c.log
		cmd.Stderr = &c.log
		if err := cmd.Start(); err != nil {
			s.setState(c, "error", fmt.Errorf("start command: %w", err))
			return
		}
		processDone = make(chan error, 1)
		go func(done chan<- error) {
			err := cmd.Wait()
			if err != nil {
				cancelRun(fmt.Errorf("start command exited: %w; %s", err, c.log.String()))
			}
			done <- err
		}(processDone)
		defer func() {
			if err := stopProcess(cmd, processDone); err != nil {
				s.setState(c, "error", fmt.Errorf("terminate capture process: %w", err))
			}
		}()
	}
	var conn *websocket.Conn
	if d.Endpoint != "" {
		var err error
		conn, err = dialCapture(runCtx, d.Endpoint, d.Start != "")
		if err != nil {
			if ctx.Err() != nil {
				s.setState(c, "stopped", nil)
			} else if cause := context.Cause(runCtx); cause != nil {
				s.setState(c, "error", cause)
			} else {
				s.setState(c, "error", fmt.Errorf("connect eCapture: %w", err))
			}
			return
		}
	}
	s.setState(c, "running", nil)
	messages := make(chan error, 1)
	if conn != nil {
		go func() { messages <- s.readCapture(conn, c, d.ID) }()
		defer func() {
			_ = conn.Close()
			if messages != nil {
				<-messages
			}
		}()
	}
	for {
		select {
		case <-ctx.Done():
			s.setState(c, "stopped", nil)
			return
		case err := <-messages:
			messages = nil
			s.setState(c, "error", err)
			return
		case err := <-processDone:
			processDone = nil // The command was reaped; it may be a one-shot launcher.
			if err != nil {
				s.setState(c, "error", fmt.Errorf("start command exited: %w; %s", err, c.log.String()))
				return
			}
			if conn == nil {
				s.setState(c, "stopped", nil)
				return
			}
		}
	}
}

func dialCapture(ctx context.Context, endpoint string, starting bool) (*websocket.Conn, error) {
	// A managed capture process binds its server after exec. Only connection-refused
	// during this explicit startup window is retried; auth/protocol errors surface.
	deadline := time.Now().Add(10 * time.Second)
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	originScheme := "http"
	if u.Scheme == "wss" {
		originScheme = "https"
	}
	headers := http.Header{"Origin": []string{originScheme + "://" + u.Host}}
	for {
		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, endpoint, headers)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err == nil {
			conn.SetReadLimit(32 << 20)
			return conn, nil
		}
		if !starting || !errors.Is(err, syscall.ECONNREFUSED) || time.Now().After(deadline) {
			return nil, err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func (s *Service) readCapture(conn *websocket.Conn, c *capture, id string) error {
	for {
		kind, b, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("eCapture stream disconnected: %w", err)
		}
		if kind != websocket.BinaryMessage {
			return fmt.Errorf("expected binary eCaptureQ Protobuf, got WebSocket type %d", kind)
		}
		event, log, err := decodeFrame(b, id)
		if err != nil {
			return fmt.Errorf("decode eCapture event: %w", err)
		}
		if log != "" {
			_, _ = c.log.Write([]byte(log + "\n"))
		}
		if event == nil {
			continue
		}
		p, _ := json.Marshal(map[string]any{"events": []any{event}})
		if _, err := s.core.Call(context.Background(), "events.ingest", p); err != nil {
			return fmt.Errorf("ingest eCapture event: %w", err)
		}
	}
}
func (s *Service) stop(ctx context.Context, id string) (any, error) {
	s.mu.Lock()
	c, ok := s.captures[id]
	s.mu.Unlock()
	if !ok {
		return CaptureStatus{DeviceID: id, State: "stopped"}, nil
	}
	c.stopOnce.Do(func() {
		// Use the configuration bound at start, even if its device entry was edited.
		if c.stopCommand != "" {
			command := shellCommand(ctx, c.stopCommand)
			command.Stdout = &c.log
			command.Stderr = &c.log
			c.stopErr = command.Run()
		}
		c.cancel()
		<-c.done
		if c.stopErr != nil {
			s.setState(c, "error", fmt.Errorf("stop command: %w; %s", c.stopErr, c.log.String()))
		}
	})
	if c.stopErr != nil {
		return nil, c.stopErr
	}
	return s.status(id)[0], nil
}
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd.exe", "/C", command)
	}
	shell := "/bin/sh"
	if runtime.GOOS == "android" {
		shell = "/system/bin/sh"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	configureProcess(cmd)
	return cmd
}
func discoverDevices(ctx context.Context) (any, error) {
	b, err := exec.CommandContext(ctx, "adb", "devices", "-l").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("adb devices: %w: %s", err, b)
	}
	out := make([]map[string]string, 0)
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] == "List" || strings.HasPrefix(f[0], "*") {
			continue
		}
		d := map[string]string{"id": f[0], "state": f[1]}
		for _, v := range f[2:] {
			if k, v, ok := strings.Cut(v, ":"); ok {
				d[k] = v
			}
		}
		out = append(out, d)
	}
	return out, nil
}

type boundedLog struct {
	mu sync.Mutex
	b  []byte
}

func (l *boundedLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b = append(l.b, b...)
	if len(l.b) > 32768 {
		l.b = bytes.Clone(l.b[len(l.b)-32768:])
	}
	return len(b), nil
}
func (l *boundedLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return string(l.b) }
