package gateway

import (
	"encoding/base64"
	"google.golang.org/protobuf/encoding/protowire"
	"testing"
)

func fieldBytes(b []byte, n protowire.Number, s string) []byte {
	b = protowire.AppendTag(b, n, protowire.BytesType)
	return protowire.AppendString(b, s)
}
func fieldInt(b []byte, n protowire.Number, v uint64) []byte {
	b = protowire.AppendTag(b, n, protowire.VarintType)
	return protowire.AppendVarint(b, v)
}
func eventFrame(payload string, format uint64) []byte {
	e := fieldInt(nil, 1, 1793624400)
	e = fieldBytes(e, 2, "conn")
	e = fieldBytes(e, 11, payload)
	e = fieldInt(e, 12, format)
	e = fieldBytes(e, 14, "request")
	e = fieldBytes(e, 16, "3")
	e = fieldInt(e, 17, 42)
	b := fieldInt(nil, 1, 2)
	return fieldBytes(b, 2, string(e))
}
func TestWireReconstructedHTTP2(t *testing.T) {
	raw := "GET /hello HTTP/2.0\r\nHost: example.org\r\n\r\n"
	e, log, err := decodeFrame(eventFrame("[2026-11-02T13:00:00Z] http/2 request 1:2 -> 3:4 host=\"example.org\"\n"+raw+"\n\n", 1), "pixel")
	if err != nil {
		t.Fatal(err)
	}
	if log != "" || e["stream_id"] != "3" || e["device_id"] != "pixel" || e["protocol"] != "http/2" {
		t.Fatalf("bad metadata: %#v %q", e, log)
	}
	if e["timestamp"] != "2026-11-02T13:00:00Z" {
		t.Fatalf("timestamp: %v", e["timestamp"])
	}
	p, _ := base64.StdEncoding.DecodeString(e["payload"].(string))
	if string(p) != raw+"\n\n" {
		t.Fatalf("payload %q", p)
	}
}
func TestWireTimestampUsesSeconds(t *testing.T) {
	e, _, err := decodeFrame(eventFrame("GET / HTTP/1.1\r\nHost: x\r\n\r\n", 1), "d")
	if err != nil {
		t.Fatal(err)
	}
	if e["timestamp"] != "2026-11-02T13:00:00Z" {
		t.Fatalf("timestamp: %v", e["timestamp"])
	}
}
func TestWireBoundaryErrors(t *testing.T) {
	for _, b := range [][]byte{{0xff}, eventFrame("secret", 2), eventFrame("packet", 3)} {
		if _, _, err := decodeFrame(b, "d"); err == nil {
			t.Fatalf("accepted %x", b)
		}
	}
	b := fieldInt(nil, 1, 1)
	b = fieldBytes(b, 4, "upstream log")
	e, l, err := decodeFrame(b, "d")
	if err != nil || e != nil || l != "upstream log" {
		t.Fatalf("log: %v %q %v", e, l, err)
	}
}
