package lens

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestBodyFormatsAndGRPC(t *testing.T) {
	protoBody := []byte{0x0a, 0x03, 'a', 'b', 'c', 0x10, 0x2a}
	out, err := decodeProtobuf(protoBody, nil, "")
	if err != nil || !strings.Contains(out, `"text": "abc"`) {
		t.Fatalf("%s %v", out, err)
	}
	frame := append([]byte{0, 0, 0, 0, byte(len(protoBody))}, protoBody...)
	if _, err := decodeGRPC(append(frame, 0), "", nil, ""); err == nil {
		t.Fatal("truncated frame accepted")
	}
	if _, err := decodeProtobuf([]byte{0xff}, nil, ""); err == nil {
		t.Fatal("invalid protobuf accepted")
	}
	var zipped bytes.Buffer
	w := gzip.NewWriter(&zipped)
	if _, err := w.Write(protoBody); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	compressed := make([]byte, 5)
	compressed[0] = 1
	binary.BigEndian.PutUint32(compressed[1:], uint32(zipped.Len()))
	compressed = append(compressed, zipped.Bytes()...)
	if _, err := decodeGRPC(compressed, "gzip", nil, ""); err != nil {
		t.Fatal(err)
	}
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("test.proto"), Package: proto.String("test"), Syntax: proto.String("proto3"), MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Reply"), Field: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("name"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()}, {Name: proto.String("count"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()}}}}}}}
	desc, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	out, err = decodeProtobuf(protoBody, desc, "test.Reply")
	if err != nil || !strings.Contains(out, `"name"`) || !strings.Contains(out, `"count"`) {
		t.Fatalf("%s %v", out, err)
	}
	if _, err = decodeProtobuf(protoBody, desc, ""); err == nil {
		t.Fatal("missing message type accepted")
	}
}

func TestHistoryRoundtripAndCustomDecoder(t *testing.T) {
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	events := []Event{event("a", "c", "", "request", "GET / HTTP/1.1\r\nHost: a\r\n\r\n"), event("a", "c", "", "response", "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 7\r\n\r\n{\"a\":1}")}
	call(t, e, "events.ingest", map[string]any{"events": events})
	id := call(t, e, "requests.list", map[string]any{}).(ListResult).Items[0].ID
	out := call(t, e, "body.view", map[string]any{"id": id, "side": "response", "format": "json"}).(map[string]string)
	if !strings.Contains(out["text"], "\n") {
		t.Fatal(out)
	}
	call(t, e, "decoders.put", map[string]any{"name": "passthrough", "command": "cat"})
	custom := call(t, e, "body.view", map[string]any{"id": id, "side": "response", "format": "passthrough"}).(map[string]string)
	if custom["text"] != "{\"a\":1}" {
		t.Fatal(custom)
	}
	for _, format := range []string{"json", "har"} {
		content := call(t, e, "history.export", map[string]any{"format": format}).(map[string]string)["content"]
		call(t, e, "requests.clear", map[string]any{})
		call(t, e, "history.import", map[string]any{"format": format, "content": content})
		items := call(t, e, "requests.list", map[string]any{}).(ListResult).Items
		if len(items) != 1 || string(items[0].Response.Body) != "{\"a\":1}" || items[0].Status != 200 {
			t.Fatalf("%s: %+v", format, items)
		}
	}
	call(t, e, "decoders.put", map[string]any{"name": "fail", "command": "exit 7"})
	b, _ := json.Marshal(map[string]any{"id": call(t, e, "requests.list", map[string]any{}).(ListResult).Items[0].ID, "side": "response", "format": "fail"})
	if _, err = e.Call(context.Background(), "body.view", b); err == nil {
		t.Fatal("decoder failure hidden")
	}
}

func TestHEADAndUnframedHTTP2Body(t *testing.T) {
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	events := []Event{event("a", "head", "", "request", "HEAD / HTTP/1.1\r\nHost: a\r\n\r\n"), event("a", "head", "", "response", "HTTP/1.1 200 OK\r\nContent-Length: 500\r\n\r\n"), event("a", "h2", "1", "request", "POST /foo HTTP/2.0\r\nHost: a\r\nContent-Type: application/json\r\n\r\n{\"b\":2}"), event("a", "h2", "1", "response", "HTTP/2.0 200\r\nContent-Length: 2\r\n\r\n{}")}
	call(t, e, "events.ingest", map[string]any{"events": events})
	items := call(t, e, "requests.list", map[string]any{}).(ListResult).Items
	if len(items) != 2 {
		t.Fatal(items)
	}
	for _, v := range items {
		if v.ConnectionID == "h2" && string(v.Request.Body) != "{\"b\":2}" {
			t.Fatalf("unframed body: %s", v.Request.Body)
		}
		if v.ConnectionID == "head" && len(v.Response.Body) != 0 {
			t.Fatal(v)
		}
	}
}

func TestBrotliBodyAndMalformedCompression(t *testing.T) {
	var b bytes.Buffer
	w := brotli.NewWriter(&b)
	if _, err := w.Write([]byte(`{"br":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	decoded, err := decompress(b.Bytes(), "br")
	if err != nil || string(decoded) != `{"br":true}` {
		t.Fatalf("%s %v", decoded, err)
	}
	if _, err = decompress([]byte("invalid"), "gzip"); err == nil {
		t.Fatal("invalid compression accepted")
	}
}

func TestReconstructedHTTP2WithoutAuthorityPreservesTarget(t *testing.T) {
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	v := event("a", "h2", "1", "request", "GET /profile?detail=1 HTTP/2.0\r\nUser-Agent: APP\r\n\r\n")
	v.Dst = "192.0.2.1:443"
	call(t, e, "events.ingest", map[string]any{"events": []Event{v}})
	items := call(t, e, "requests.list", map[string]any{}).(ListResult).Items
	if len(items) != 1 || items[0].URL != "/profile?detail=1" || items[0].Dst != v.Dst {
		t.Fatalf("lost request target or endpoint: %+v", items)
	}
}

func TestGzipBody(t *testing.T) {
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err = w.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", b.Len())), b.Bytes()...)
	ev := event("a", "c", "", "response", string(payload))
	call(t, e, "events.ingest", map[string]any{"events": []Event{ev}})
	id := call(t, e, "requests.list", map[string]any{}).(ListResult).Items[0].ID
	out := call(t, e, "body.view", map[string]any{"id": id, "side": "response", "format": "json"}).(map[string]string)
	if !strings.Contains(out["text"], `"ok": true`) {
		t.Fatal(out)
	}
}
