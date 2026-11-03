package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRemoteCommandsAndOutput(t *testing.T) {
	var method string
	var params map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("request %s auth %q", r.URL, r.Header.Get("Authorization"))
		}
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		method, params = req.Method, req.Params
		fmt.Fprint(w, `{"result":{"text":"{\n  \"ok\": true\n}"}}`)
	}))
	defer srv.Close()
	var out, errout bytes.Buffer
	args := []string{"body", "view", "r1", "--side", "response", "--format", "json", "--server", srv.URL, "--token", "test-token"}
	if err := run(context.Background(), args, strings.NewReader(""), &out, &errout); err != nil {
		t.Fatal(err)
	}
	if method != "body.view" || params["id"] != "r1" || params["side"] != "response" || params["format"] != "json" {
		t.Fatalf("%s %#v", method, params)
	}
	if out.String() != "{\n  \"ok\": true\n}\n" {
		t.Fatalf("stdout %q", out.String())
	}
}

func TestRemoteErrorsAreNotSuccessfulOutput(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":"unknown request"}`)
			}))
			defer srv.Close()
			var out bytes.Buffer
			err := run(context.Background(), []string{"--server", srv.URL, "requests", "get", "missing"}, strings.NewReader(""), &out, &out)
			if err == nil || !strings.Contains(err.Error(), "unknown request") {
				t.Fatalf("error %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("unexpected stdout %q", out.String())
			}
		})
	}
}

func TestJSONLEventIngest(t *testing.T) {
	var events []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Events []any `json:"events"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		events = req.Params.Events
		fmt.Fprint(w, `{"result":{"ingested":2}}`)
	}))
	defer srv.Close()
	var out bytes.Buffer
	err := run(context.Background(), []string{"ingest", "--file", "-", "--server", srv.URL}, strings.NewReader("{\"device_id\":\"a\"}\n{\"device_id\":\"b\"}\n"), &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events %#v", events)
	}
}

func TestExportFileAndParameterValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"content":"[1,2]","mime":"application/json"}}`)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "history.json")
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--server", srv.URL, "history", "export", "--output", dest}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != "[1,2]" {
		t.Fatalf("file %q err %v", b, err)
	}
	if out.Len() != 0 {
		t.Fatalf("export stdout %q", out.String())
	}
	if err := run(context.Background(), []string{"requests", "get"}, strings.NewReader(""), &out, &out); err == nil {
		t.Fatal("missing id accepted")
	}
	if err := run(context.Background(), []string{"rpc", "requests.list", "--params", "bad"}, strings.NewReader(""), &out, &out); err == nil {
		t.Fatal("invalid params accepted")
	}
}

func TestServerBindingRequiresToken(t *testing.T) {
	if err := validateListen("0.0.0.0:8787", ""); err == nil {
		t.Fatal("unauthenticated network bind accepted")
	}
	if err := validateListen("127.0.0.1:8787", ""); err != nil {
		t.Fatal(err)
	}
	if err := validateListen("[::1]:8787", ""); err != nil {
		t.Fatal(err)
	}
	if err := validateListen(":8787", "secret"); err != nil {
		t.Fatal(err)
	}
}

func TestForegroundCaptureSurfacesAsyncProcessError(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--data-dir", dir, "devices", "put", "broken", "--start-command", "exit 7"}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := run(ctx, []string{"--data-dir", dir, "capture", "start", "broken"}, strings.NewReader(""), &out, &out)
	if err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("capture error %v, output %q", err, out.String())
	}
}

func TestWebCommandServesBundleAndAuthenticatedAPI(t *testing.T) {
	for _, tc := range []struct {
		name, token, listen string
		defaultDir          bool
	}{
		{"generated token on wildcard listener", "", ":0", false},
		{"provided token", "explicit +/#?token", "127.0.0.1:0", false},
		{"default assets beside executable", "secret", "127.0.0.1:0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.defaultDir {
				executable, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				dir = filepath.Join(filepath.Dir(executable), "web")
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.RemoveAll(dir) })
			}
			if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>Lens</html>"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			reader, writer := io.Pipe()
			done := make(chan error, 1)
			args := []string{"web", "--listen", tc.listen, "--data-dir", t.TempDir()}
			if !tc.defaultDir {
				args = append(args, "--web-dir", dir)
			}
			if tc.token != "" {
				args = append(args, "--token", tc.token)
			}
			go func() {
				err := run(ctx, args, strings.NewReader(""), writer, io.Discard)
				writer.CloseWithError(err)
				done <- err
			}()
			t.Cleanup(func() {
				cancel()
				reader.Close()
				if err := <-done; err != nil {
					t.Error(err)
				}
			})
			var startup struct {
				URL           string `json:"url"`
				Authenticated bool   `json:"authenticated"`
			}
			if err := json.NewDecoder(reader).Decode(&startup); err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(startup.URL)
			if err != nil || !startup.Authenticated || u.Path != "/" {
				t.Fatalf("startup %+v, %v", startup, err)
			}
			_, rawFragment, found := strings.Cut(startup.URL, "#")
			if !found || (tc.token != "" && rawFragment != "token="+url.QueryEscape(tc.token)) {
				t.Fatalf("token query was not encoded exactly once: %q", rawFragment)
			}
			fragment, err := url.ParseQuery(rawFragment)
			if err != nil || fragment.Get("token") == "" || (tc.token != "" && fragment.Get("token") != tc.token) {
				t.Fatalf("fragment token invalid: %v", err)
			}
			u.Fragment = ""
			resp, err := http.Get(u.String())
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != 200 || string(body) != "<html>Lens</html>" {
				t.Fatalf("bundle: %d %q %v", resp.StatusCode, body, err)
			}
			_, err = remoteCall(ctx, options{server: u.String(), token: fragment.Get("token"), timeout: time.Second}, "system.info", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := remoteCall(ctx, options{server: u.String(), timeout: time.Second}, "system.info", json.RawMessage(`{}`)); err == nil {
				t.Fatal("web API accepted missing token")
			}
		})
	}
}

func TestWebCommandMissingAssetsAndRemoteMode(t *testing.T) {
	for _, args := range [][]string{
		{"web", "--web-dir", t.TempDir(), "--data-dir", t.TempDir()},
		{"web", "--server", "http://127.0.0.1:8787", "--data-dir", t.TempDir()},
	} {
		var out bytes.Buffer
		if err := run(context.Background(), args, strings.NewReader(""), &out, &out); err == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
		if out.Len() != 0 {
			t.Fatalf("failed startup emitted success: %q", out.String())
		}
	}
}
