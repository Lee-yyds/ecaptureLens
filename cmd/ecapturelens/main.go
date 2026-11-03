package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ecapturelens/gateway"
)

const version = "0.1.0"

type options struct {
	dataDir, server, token string
	json                   bool
	timeout                time.Duration
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "ecapturelens:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, in io.Reader, out, errout io.Writer) (err error) {
	opts, rest, err := globalOptions(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		_, err = io.WriteString(out, helpText)
		return err
	}
	if len(rest) > 1 && (rest[1] == "--help" || rest[1] == "-h") {
		_, err = io.WriteString(out, helpText)
		return err
	}
	if rest[0] == "version" {
		if len(rest) > 1 {
			return fmt.Errorf("version takes no arguments")
		}
		return writeJSON(out, map[string]string{"version": version})
	}
	if rest[0] == "serve" || rest[0] == "web" {
		return serve(ctx, opts, rest[0], rest[1:], out)
	}
	method, params, output, err := command(rest, in)
	if errors.Is(err, flag.ErrHelp) {
		_, err = io.WriteString(out, helpText)
		return err
	}
	if err != nil {
		return err
	}
	var result any
	if opts.server != "" {
		result, err = remoteCall(ctx, opts, method, params)
	} else {
		var service *gateway.Service
		service, err = gateway.Open(opts.dataDir)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, service.Close()) }()
		result, err = service.Call(ctx, method, params)
		if err == nil && method == "capture.start" {
			if err := renderResult(out, result, opts.json, output); err != nil {
				return err
			}
			return waitCapture(ctx, service, params)
		}
	}
	if err != nil {
		return err
	}
	return renderResult(out, result, opts.json, output)
}

func waitCapture(ctx context.Context, service *gateway.Service, params json.RawMessage) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := service.Call(stopCtx, "capture.stop", params)
			return err
		case <-ticker.C:
			value, err := service.Call(ctx, "capture.status", params)
			if err != nil {
				return err
			}
			for _, status := range value.([]gateway.CaptureStatus) {
				switch status.State {
				case "error":
					return fmt.Errorf("capture %s: %s", status.DeviceID, status.Error)
				case "stopped":
					return nil
				}
			}
		}
	}
}

func globalOptions(args []string) (options, []string, error) {
	o := options{dataDir: os.Getenv("ECAPTURELENS_DATA_DIR"), server: os.Getenv("ECAPTURELENS_SERVER"), token: os.Getenv("ECAPTURELENS_TOKEN"), timeout: 30 * time.Second}
	if o.dataDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return o, nil, err
		}
		o.dataDir = filepath.Join(dir, "ecapturelens")
	}
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "--json":
			if hasValue && value != "true" && value != "false" {
				return o, nil, fmt.Errorf("invalid --json value %q", value)
			}
			o.json = !hasValue || value == "true"
		case "--data-dir", "--server", "--token", "--timeout":
			if !hasValue {
				i++
				if i == len(args) {
					return o, nil, fmt.Errorf("%s needs a value", name)
				}
				value = args[i]
			}
			switch name {
			case "--data-dir":
				o.dataDir = value
			case "--server":
				o.server = value
			case "--token":
				o.token = value
			case "--timeout":
				var err error
				o.timeout, err = time.ParseDuration(value)
				if err != nil || o.timeout <= 0 {
					return o, nil, fmt.Errorf("--timeout must be a positive duration")
				}
			}
		default:
			rest = append(rest, arg)
		}
	}
	return o, rest, nil
}

func remoteCall(ctx context.Context, opts options, method string, params json.RawMessage) (any, error) {
	u, err := url.Parse(opts.server)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("--server must be an http(s) base URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api"
	payload, err := json.Marshal(map[string]any{"method": method, "params": params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.token != "" {
		req.Header.Set("Authorization", "Bearer "+opts.token)
	}
	client := &http.Client{Timeout: opts.timeout}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := readInput(res.Body)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("API HTTP %d returned invalid JSON: %w", res.StatusCode, err)
	}
	if envelope.Error != "" {
		return nil, errors.New(envelope.Error)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("API HTTP %d", res.StatusCode)
	}
	if len(envelope.Result) == 0 {
		return nil, fmt.Errorf("API response has no result")
	}
	return envelope.Result, nil
}

func serve(ctx context.Context, opts options, mode string, args []string, out io.Writer) (err error) {
	if opts.server != "" {
		return fmt.Errorf("%s runs locally; remove --server or ECAPTURELENS_SERVER", mode)
	}
	fs := newFlags(mode)
	listen := fs.String("listen", "127.0.0.1:8787", "HTTP listen address")
	var webDir string
	if mode == "web" {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		fs.StringVar(&webDir, "web-dir", filepath.Join(filepath.Dir(executable), "web"), "Flutter web bundle directory")
	}
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err = io.WriteString(out, helpText)
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s takes no positional arguments", mode)
	}
	if mode == "web" && opts.token == "" {
		var secret [32]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return err
		}
		opts.token = hex.EncodeToString(secret[:])
	}
	if err := validateListen(*listen, opts.token); err != nil {
		return err
	}
	svc, err := gateway.Open(opts.dataDir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, svc.Close()) }()
	handler := svc.Handler(opts.token)
	if mode == "web" {
		handler, err = svc.WebHandler(opts.token, webDir)
		if err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	address := listener.Addr().(*net.TCPAddr)
	host := address.IP.String()
	if address.IP.IsUnspecified() {
		host = "127.0.0.1"
		if address.IP.To4() == nil {
			host = "::1"
		}
	}
	serviceURL := url.URL{Scheme: "http", Host: net.JoinHostPort(host, fmt.Sprint(address.Port))}
	launchURL := serviceURL.String()
	if mode == "web" {
		launchURL += "/#token=" + url.QueryEscape(opts.token)
	}
	if err := writeJSON(out, map[string]any{"url": launchURL, "data_dir": opts.dataDir, "authenticated": opts.token != ""}); err != nil {
		return errors.Join(err, listener.Close())
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			shutdownErr = errors.Join(shutdownErr, server.Close())
		}
		serveErr := <-done
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(shutdownErr, serveErr)
	}
}

func validateListen(address, token string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if token == "" && host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("non-loopback --listen requires --token (or ECAPTURELENS_TOKEN)")
	}
	return nil
}

type resultOutput struct{ field, file string }

func renderResult(out io.Writer, result any, asJSON bool, output resultOutput) error {
	if output.field == "" || (asJSON && (output.file == "" || output.file == "-")) {
		return writeJSON(out, result)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	var content string
	if err := json.Unmarshal(object[output.field], &content); err != nil {
		return fmt.Errorf("result.%s: %w", output.field, err)
	}
	if output.file != "" && output.file != "-" {
		return os.WriteFile(output.file, []byte(content), 0600)
	}
	if _, err := io.WriteString(out, content); err != nil {
		return err
	}
	if !strings.HasSuffix(content, "\n") {
		_, err = io.WriteString(out, "\n")
	}
	return err
}

func writeJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
}

func readInput(in io.Reader) ([]byte, error) {
	const maxInput = 128 << 20
	b, err := io.ReadAll(io.LimitReader(in, maxInput+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxInput {
		return nil, fmt.Errorf("input exceeds 128 MiB")
	}
	return b, nil
}
