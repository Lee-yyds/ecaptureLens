package lens

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func decompress(body []byte, encoding string) ([]byte, error) {
	encoding = strings.ToLower(strings.TrimSpace(encoding))
	if encoding == "" || encoding == "identity" {
		return body, nil
	}
	if strings.Contains(encoding, ",") {
		parts := strings.Split(encoding, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			var err error
			body, err = decompress(body, parts[i])
			if err != nil {
				return nil, err
			}
		}
		return body, nil
	}
	var r io.ReadCloser
	var err error
	switch encoding {
	case "gzip":
		r, err = gzip.NewReader(bytes.NewReader(body))
	case "deflate":
		r, err = zlib.NewReader(bytes.NewReader(body))
	case "br":
		r = io.NopCloser(brotli.NewReader(bytes.NewReader(body)))
	default:
		return nil, fmt.Errorf("unsupported content encoding %q; raw bytes remain available in hex and export", encoding)
	}
	if err != nil {
		return nil, err
	}
	const maxDecoded = 64 << 20
	b, err := io.ReadAll(io.LimitReader(r, maxDecoded+1))
	err = errors.Join(err, r.Close())
	if err != nil {
		return nil, err
	}
	if len(b) > maxDecoded {
		return nil, errors.New("decoded body exceeds 64 MiB")
	}
	return b, nil
}

func viewBody(ctx context.Context, raw json.RawMessage, snapshot state) (any, error) {
	var p struct {
		ID          string `json:"id"`
		Side        string `json:"side"`
		Format      string `json:"format"`
		Descriptor  []byte `json:"descriptor"`
		MessageType string `json:"message_type"`
	}
	if err := params(raw, &p); err != nil {
		return nil, err
	}
	var t *Transaction
	for i := range snapshot.Requests {
		if snapshot.Requests[i].ID == p.ID {
			t = &snapshot.Requests[i]
			break
		}
	}
	if t == nil {
		return nil, fmt.Errorf("request %q not found", p.ID)
	}
	var m *Message
	switch p.Side {
	case "request":
		m = t.Request
	case "response":
		m = t.Response
	default:
		return nil, errors.New("side must be request or response")
	}
	if m == nil {
		return nil, fmt.Errorf("%s is not captured", p.Side)
	}
	if p.Format == "hex" {
		return map[string]string{"text": hex.Dump(m.Body)}, nil
	}
	body, err := decompress(m.Body, m.Headers.Get("Content-Encoding"))
	if err != nil {
		return nil, err
	}
	var text string
	switch p.Format {
	case "text":
		if !utf8.Valid(body) {
			return nil, errors.New("body is not UTF-8; use hex")
		}
		text = string(body)
	case "json":
		var out bytes.Buffer
		if err = json.Indent(&out, body, "", "  "); err != nil {
			return nil, err
		}
		text = out.String()
	case "protobuf":
		text, err = decodeProtobuf(body, p.Descriptor, p.MessageType)
	case "grpc":
		text, err = decodeGRPC(body, m.Headers.Get("Grpc-Encoding"), p.Descriptor, p.MessageType)
	default:
		var command string
		for _, d := range snapshot.Decoders {
			if d.Name == p.Format {
				command = d.Command
				break
			}
		}
		if command == "" {
			return nil, fmt.Errorf("unknown body format %q", p.Format)
		}
		shell := "sh"
		args := []string{"-c", command}
		if runtime.GOOS == "android" {
			shell = "/system/bin/sh"
		}
		if runtime.GOOS == "windows" {
			shell = "cmd"
			args = []string{"/C", command}
		}
		cmd := exec.CommandContext(ctx, shell, args...)
		cmd.Stdin = bytes.NewReader(body)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, runErr := cmd.Output()
		if runErr != nil {
			return nil, fmt.Errorf("decoder %s: %w: %s", p.Format, runErr, strings.TrimSpace(stderr.String()))
		}
		if !utf8.Valid(out) {
			return nil, errors.New("decoder output is not UTF-8")
		}
		text = string(out)
	}
	if err != nil {
		return nil, err
	}
	return map[string]string{"text": text}, nil
}

func decodeProtobuf(body, descriptor []byte, messageType string) (string, error) {
	if len(descriptor) > 0 {
		if messageType == "" {
			return "", errors.New("message_type is required with descriptor")
		}
		var set descriptorpb.FileDescriptorSet
		if err := proto.Unmarshal(descriptor, &set); err != nil {
			return "", err
		}
		files, err := protodesc.NewFiles(&set)
		if err != nil {
			return "", err
		}
		d, err := files.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(messageType, ".")))
		if err != nil {
			return "", err
		}
		md, ok := d.(protoreflect.MessageDescriptor)
		if !ok {
			return "", errors.New("message_type is not a message")
		}
		m := dynamicpb.NewMessage(md)
		if err = proto.Unmarshal(body, m); err != nil {
			return "", err
		}
		out, err := protojson.MarshalOptions{Indent: "  ", UseProtoNames: true}.Marshal(m)
		return string(out), err
	}
	if messageType != "" {
		return "", errors.New("descriptor is required with message_type")
	}
	fields := []map[string]any{}
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return "", protowire.ParseError(n)
		}
		if !num.IsValid() {
			return "", errors.New("invalid protobuf field number")
		}
		body = body[n:]
		field := map[string]any{"field": num, "wire_type": int(typ)}
		var consumed int
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(body)
			consumed = n
			field["value"] = fmt.Sprint(v)
		case protowire.Fixed32Type:
			v, n := protowire.ConsumeFixed32(body)
			consumed = n
			field["value"] = v
		case protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(body)
			consumed = n
			field["value"] = fmt.Sprint(v)
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(body)
			consumed = n
			field["base64"] = base64.StdEncoding.EncodeToString(v)
			if utf8.Valid(v) {
				field["text"] = string(v)
			}
		default:
			return "", fmt.Errorf("protobuf wire type %d requires a descriptor or is invalid", typ)
		}
		if consumed < 0 {
			return "", protowire.ParseError(consumed)
		}
		fields = append(fields, field)
		body = body[consumed:]
	}
	out, err := json.MarshalIndent(fields, "", "  ")
	return string(out), err
}

func decodeGRPC(body []byte, encoding string, descriptor []byte, messageType string) (string, error) {
	frames := []json.RawMessage{}
	for len(body) > 0 {
		if len(body) < 5 {
			return "", errors.New("truncated gRPC frame header")
		}
		compressed := body[0]
		length := uint64(binary.BigEndian.Uint32(body[1:5]))
		body = body[5:]
		if length > uint64(len(body)) {
			return "", errors.New("truncated gRPC message")
		}
		data := body[:int(length)]
		body = body[int(length):]
		if compressed > 1 {
			return "", fmt.Errorf("invalid gRPC compression flag %d", compressed)
		}
		if compressed == 1 {
			if encoding == "" || encoding == "identity" {
				return "", errors.New("compressed gRPC message requires grpc-encoding")
			}
			var err error
			data, err = decompress(data, encoding)
			if err != nil {
				return "", err
			}
		}
		decoded, err := decodeProtobuf(data, descriptor, messageType)
		if err != nil {
			return "", err
		}
		frames = append(frames, json.RawMessage(decoded))
	}
	out, err := json.MarshalIndent(frames, "", "  ")
	return string(out), err
}
