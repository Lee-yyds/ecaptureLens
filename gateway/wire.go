package gateway

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

// decodeFrame consumes upstream LogEntry field numbers, including the v2.9
// metadata. Unknown protobuf fields are skipped according to the wire contract.
func decodeFrame(b []byte, device string) (map[string]any, string, error) {
	fields, err := wireFields(b)
	if err != nil {
		return nil, "", err
	}
	kind, _ := fields[1].(uint64)
	if kind == 0 {
		return nil, "", nil
	}
	if kind == 1 {
		log, _ := fields[4].([]byte)
		return nil, string(log), nil
	}
	if kind != 2 {
		return nil, "", fmt.Errorf("unknown eCapture log type %d", kind)
	}
	event, ok := fields[2].([]byte)
	if !ok {
		return nil, "", fmt.Errorf("eCapture EVENT missing event_payload")
	}
	f, err := wireFields(event)
	if err != nil {
		return nil, "", err
	}
	number := func(n int) uint64 { v, _ := f[n].(uint64); return v }
	str := func(n int) string { v, _ := f[n].([]byte); return string(v) }
	if format := number(12); format > 1 {
		return nil, "", fmt.Errorf("capture format %d is not reconstructed text; use eCapture tls --text-source pcapng with --ecaptureq", format)
	}
	payload, ok := f[11].([]byte)
	if !ok {
		return nil, "", fmt.Errorf("eCapture event missing payload")
	}
	protocol := ""
	var preciseTime time.Time
	if bytes.HasPrefix(payload, []byte("[")) {
		if end := bytes.IndexByte(payload, '\n'); end >= 0 {
			prefix := strings.Fields(string(payload[:end]))
			if len(prefix) >= 3 && strings.HasPrefix(prefix[1], "http/") {
				protocol = prefix[1]
				preciseTime, err = time.Parse(time.RFC3339Nano, strings.Trim(prefix[0], "[]"))
				if err != nil {
					return nil, "", fmt.Errorf("invalid reconstructed timestamp: %w", err)
				}
				payload = payload[end+1:]
			}
		}
	}
	direction := str(14)
	if direction == "" {
		if bytes.HasPrefix(payload, []byte("HTTP/")) {
			direction = "response"
		} else if bytes.Contains(bytes.SplitN(payload, []byte("\n"), 2)[0], []byte(" HTTP/")) {
			direction = "request"
		} else {
			return nil, "", fmt.Errorf("event has no direction or reconstructed HTTP start line; use --text-source pcapng")
		}
	}
	timestamp := number(1)
	if timestamp == 0 {
		return nil, "", fmt.Errorf("event missing timestamp")
	}
	// Upstream Event.timestamp uses Unix seconds. Reconstructed payload carries
	// nanosecond precision; preserve it for useful request/response durations.
	if preciseTime.IsZero() {
		preciseTime = time.Unix(int64(timestamp), 0)
	}
	e := map[string]any{"device_id": device, "connection_id": str(2), "stream_id": str(16), "timestamp": preciseTime.UTC().Format(time.RFC3339Nano), "direction": direction, "protocol": protocol, "payload": base64.StdEncoding.EncodeToString(payload), "src": net.JoinHostPort(str(3), strconv.FormatUint(number(4), 10)), "dst": net.JoinHostPort(str(5), strconv.FormatUint(number(6), 10)), "pid": number(7), "process": str(8), "sequence": number(17)}
	return e, "", nil
}

func wireFields(b []byte) (map[int]any, error) {
	out := make(map[int]any)
	for len(b) > 0 {
		num, kind, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		b = b[n:]
		switch kind {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			out[int(num)] = v
			b = b[n:]
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			out[int(num)] = v
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, kind, b)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return out, nil
}
