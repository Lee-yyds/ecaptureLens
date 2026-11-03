package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags accepts flags before or after positional IDs, matching CLI examples.
func parseFlags(fs *flag.FlagSet, args []string) error {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		f := fs.Lookup(name)
		if f == nil || hasValue {
			continue
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			continue
		}
		i++
		if i == len(args) {
			return fmt.Errorf("--%s needs a value", name)
		}
		flags = append(flags, args[i])
	}
	return fs.Parse(append(flags, positional...))
}

func requiredID(fs *flag.FlagSet, id string) (string, error) {
	if id == "" && fs.NArg() == 1 {
		id = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return "", fmt.Errorf("unexpected positional arguments")
	}
	if id == "" {
		return "", fmt.Errorf("--id or positional ID is required")
	}
	return id, nil
}

func command(args []string, in io.Reader) (string, json.RawMessage, resultOutput, error) {
	var method string
	params := map[string]any{}
	output := resultOutput{}
	if len(args) == 0 {
		return "", nil, output, fmt.Errorf("command is required")
	}
	group := args[0]
	action := ""
	tail := args[1:]
	if group != "rpc" && group != "ingest" && len(tail) > 0 {
		action, tail = tail[0], tail[1:]
	}
	fs := newFlags(group + " " + action)
	finish := func() (string, json.RawMessage, resultOutput, error) {
		b, err := json.Marshal(params)
		return method, b, output, err
	}
	noPositionals := func() error {
		if fs.NArg() > 0 {
			return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
		}
		return nil
	}
	switch group {
	case "requests":
		switch action {
		case "list":
			query := fs.String("query", "", "Search method, URL, status and body")
			device := fs.String("device", "", "Device ID")
			conn := fs.String("connection", "", "Connection ID")
			limit := fs.Int("limit", 1000, "Maximum results")
			offset := fs.Int("offset", 0, "Result offset")
			if err := parseFlags(fs, tail); err != nil {
				return "", nil, output, err
			}
			if err := noPositionals(); err != nil {
				return "", nil, output, err
			}
			if *limit < 1 || *offset < 0 {
				return "", nil, output, fmt.Errorf("--limit must be positive and --offset nonnegative")
			}
			method = "requests.list"
			params = map[string]any{"query": *query, "device_id": *device, "connection_id": *conn, "limit": *limit, "offset": *offset}
		case "get":
			id := fs.String("id", "", "Request ID")
			if err := parseFlags(fs, tail); err != nil {
				return "", nil, output, err
			}
			value, err := requiredID(fs, *id)
			if err != nil {
				return "", nil, output, err
			}
			method, params["id"] = "requests.get", value
		case "clear":
			if err := parseFlags(fs, tail); err != nil {
				return "", nil, output, err
			}
			if err := noPositionals(); err != nil {
				return "", nil, output, err
			}
			method = "requests.clear"
		default:
			return "", nil, output, fmt.Errorf("requests expects list, get or clear")
		}
	case "body":
		if action != "view" {
			return "", nil, output, fmt.Errorf("body expects view")
		}
		id := fs.String("id", "", "Request ID")
		side := fs.String("side", "response", "request or response")
		format := fs.String("format", "text", "text, json, hex, protobuf, grpc or a decoder name")
		descriptor := fs.String("descriptor", "", "Protobuf FileDescriptorSet file")
		messageType := fs.String("message-type", "", "Fully qualified protobuf message name")
		file := fs.String("output", "", "Output file (stdout by default)")
		if err := parseFlags(fs, tail); err != nil {
			return "", nil, output, err
		}
		value, err := requiredID(fs, *id)
		if err != nil {
			return "", nil, output, err
		}
		if *side != "request" && *side != "response" {
			return "", nil, output, fmt.Errorf("--side must be request or response")
		}
		params = map[string]any{"id": value, "side": *side, "format": *format, "message_type": *messageType}
		if *descriptor != "" {
			b, err := os.ReadFile(*descriptor)
			if err != nil {
				return "", nil, output, err
			}
			params["descriptor"] = base64.StdEncoding.EncodeToString(b)
		}
		method, output = "body.view", resultOutput{field: "text", file: *file}
	case "sessions":
		device := fs.String("device", "", "Device ID")
		conn := fs.String("connection", "", "Connection ID for timeline")
		if err := parseFlags(fs, tail); err != nil {
			return "", nil, output, err
		}
		params["device_id"] = *device
		switch action {
		case "list":
			if err := noPositionals(); err != nil {
				return "", nil, output, err
			}
			method = "sessions.list"
		case "timeline":
			value, err := requiredID(fs, *conn)
			if err != nil {
				return "", nil, output, err
			}
			method, params["connection_id"] = "sessions.timeline", value
		default:
			return "", nil, output, fmt.Errorf("sessions expects list or timeline")
		}
	case "history":
		format := fs.String("format", "json", "json or har")
		fileFlag := "output"
		if action == "import" {
			fileFlag = "file"
		}
		file := fs.String(fileFlag, "-", "File or - for stdio")
		ids := fs.String("ids", "", "Comma-separated request IDs to export")
		if err := parseFlags(fs, tail); err != nil {
			return "", nil, output, err
		}
		if err := noPositionals(); err != nil {
			return "", nil, output, err
		}
		if *format != "json" && *format != "har" {
			return "", nil, output, fmt.Errorf("--format must be json or har")
		}
		params["format"] = *format
		switch action {
		case "export":
			method, output = "history.export", resultOutput{field: "content", file: *file}
			if *ids != "" {
				params["ids"] = strings.Split(*ids, ",")
			}
		case "import":
			b, err := readFile(*file, in)
			if err != nil {
				return "", nil, output, err
			}
			method, params["content"] = "history.import", string(b)
		default:
			return "", nil, output, fmt.Errorf("history expects import or export")
		}
	case "devices":
		switch action {
		case "list", "discover":
			if err := parseFlags(fs, tail); err != nil {
				return "", nil, output, err
			}
			if err := noPositionals(); err != nil {
				return "", nil, output, err
			}
			method = "devices." + action
		case "get", "delete":
			id := fs.String("id", "", "Device ID")
			if err := parseFlags(fs, tail); err != nil {
				return "", nil, output, err
			}
			value, err := requiredID(fs, *id)
			if err != nil {
				return "", nil, output, err
			}
			method, params["id"] = "devices."+action, value
		case "put":
			id := fs.String("id", "", "Device ID")
			name := fs.String("name", "", "Display name")
			endpoint := fs.String("endpoint", "", "eCaptureQ WebSocket URL")
			start := fs.String("start-command", "", "Explicit capture start shell command")
			stop := fs.String("stop-command", "", "Explicit capture stop shell command")
			if err := parseFlags(fs, tail); err != nil {
				return "", nil, output, err
			}
			value, err := requiredID(fs, *id)
			if err != nil {
				return "", nil, output, err
			}
			if *name == "" {
				*name = value
			}
			method, params = "devices.put", map[string]any{"id": value, "name": *name, "endpoint": *endpoint, "start_command": *start, "stop_command": *stop}
		default:
			return "", nil, output, fmt.Errorf("devices expects list, get, put, delete or discover")
		}
	case "capture":
		if action != "start" && action != "stop" && action != "status" {
			return "", nil, output, fmt.Errorf("capture expects start, stop or status")
		}
		device := fs.String("device", "", "Device ID")
		if err := parseFlags(fs, tail); err != nil {
			return "", nil, output, err
		}
		value := *device
		if value == "" && fs.NArg() == 1 {
			value = fs.Arg(0)
		} else if fs.NArg() != 0 {
			return "", nil, output, fmt.Errorf("unexpected positional arguments")
		}
		if value == "" && action != "status" {
			return "", nil, output, fmt.Errorf("--device or positional device ID is required")
		}
		method, params["device_id"] = "capture."+action, value
	case "decoders":
		switch action {
		case "list":
			if err := parseFlags(fs, tail); err != nil {
				return "", nil, output, err
			}
			if err := noPositionals(); err != nil {
				return "", nil, output, err
			}
			method = "decoders.list"
		case "put", "delete":
			name := fs.String("name", "", "Decoder name")
			cmd := fs.String("command", "", "Shell command: body bytes stdin, decoded UTF8 stdout")
			if err := parseFlags(fs, tail); err != nil {
				return "", nil, output, err
			}
			value, err := requiredID(fs, *name)
			if err != nil {
				return "", nil, output, err
			}
			if action == "put" && *cmd == "" {
				return "", nil, output, fmt.Errorf("--command is required")
			}
			method, params["name"] = "decoders."+action, value
			if action == "put" {
				params["command"] = *cmd
			}
		default:
			return "", nil, output, fmt.Errorf("decoders expects list, put or delete")
		}
	case "api":
		if action != "describe" {
			return "", nil, output, fmt.Errorf("api expects describe")
		}
		if err := parseFlags(fs, tail); err != nil {
			return "", nil, output, err
		}
		if err := noPositionals(); err != nil {
			return "", nil, output, err
		}
		method = "api.describe"
	case "system":
		if action != "info" {
			return "", nil, output, fmt.Errorf("system expects info")
		}
		if err := parseFlags(fs, tail); err != nil {
			return "", nil, output, err
		}
		if err := noPositionals(); err != nil {
			return "", nil, output, err
		}
		method = "system.info"
	case "rpc":
		p := fs.String("params", "{}", "JSON operation parameters")
		if err := parseFlags(fs, tail); err != nil {
			return "", nil, output, err
		}
		if fs.NArg() != 1 {
			return "", nil, output, fmt.Errorf("rpc requires one METHOD")
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(*p), &object); err != nil || object == nil {
			return "", nil, output, fmt.Errorf("--params must be a JSON object")
		}
		return fs.Arg(0), json.RawMessage(*p), output, nil
	case "ingest":
		file := fs.String("file", "-", "Event JSON, JSON array, {events:[...]} or JSONL file")
		if err := parseFlags(fs, tail); err != nil {
			return "", nil, output, err
		}
		if err := noPositionals(); err != nil {
			return "", nil, output, err
		}
		b, err := readFile(*file, in)
		if err != nil {
			return "", nil, output, err
		}
		events, err := readEvents(b)
		if err != nil {
			return "", nil, output, err
		}
		method, params["events"] = "events.ingest", events
	default:
		if group == "--help" || group == "-h" || action == "--help" || action == "-h" {
			return "", nil, output, flag.ErrHelp
		}
		return "", nil, output, fmt.Errorf("unknown command %q; use ecapturelens help", strings.Join(args, " "))
	}
	return finish()
}

func readFile(name string, in io.Reader) ([]byte, error) {
	if name == "-" {
		return readInput(in)
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readInput(f)
}

func readEvents(b []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("event input is empty")
	}
	if trimmed[0] == '[' {
		var events []json.RawMessage
		if err := json.Unmarshal(trimmed, &events); err != nil {
			return nil, err
		}
		return events, nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	var events []json.RawMessage
	for {
		var object map[string]json.RawMessage
		if err := dec.Decode(&object); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if object == nil {
			return nil, fmt.Errorf("events must be JSON objects")
		}
		if nested, ok := object["events"]; ok {
			var batch []json.RawMessage
			if err := json.Unmarshal(nested, &batch); err != nil {
				return nil, err
			}
			events = append(events, batch...)
		} else {
			encoded, err := json.Marshal(object)
			if err != nil {
				return nil, err
			}
			events = append(events, encoded)
		}
	}
	return events, nil
}
