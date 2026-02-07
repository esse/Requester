package snapshot

import (
	"encoding/base64"
	"testing"
)

func TestParseBody_JSON(t *testing.T) {
	raw := []byte(`{"name":"Alice","id":1}`)
	result := ParseBody(raw, "application/json")

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", m["name"])
	}
}

func TestParseBody_JSONRpc(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`)
	result := ParseBody(raw, "application/json-rpc")

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map for JSON-RPC body, got %T", result)
	}
	if m["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc=2.0, got %v", m["jsonrpc"])
	}
	if m["method"] != "eth_blockNumber" {
		t.Errorf("expected method=eth_blockNumber, got %v", m["method"])
	}
}

func TestParseBody_Protobuf(t *testing.T) {
	// Simulate binary protobuf data
	raw := []byte{0x08, 0x96, 0x01, 0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
	result := ParseBody(raw, "application/protobuf")

	eb, ok := result.(*EncodedBody)
	if !ok {
		t.Fatalf("expected EncodedBody for protobuf, got %T", result)
	}
	if eb.Encoding != BodyEncodingBase64 {
		t.Errorf("expected base64 encoding, got %q", eb.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(eb.Data.(string))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(raw) {
		t.Errorf("decoded length mismatch: %d vs %d", len(decoded), len(raw))
	}
}

func TestParseBody_GRPCWeb(t *testing.T) {
	raw := []byte{0x00, 0x00, 0x00, 0x00, 0x05, 0x08, 0x96, 0x01}
	result := ParseBody(raw, "application/grpc-web+proto")

	eb, ok := result.(*EncodedBody)
	if !ok {
		t.Fatalf("expected EncodedBody for gRPC-Web, got %T", result)
	}
	if eb.Encoding != BodyEncodingBase64 {
		t.Errorf("expected base64 encoding, got %q", eb.Encoding)
	}
}

func TestParseBody_PlainText(t *testing.T) {
	raw := []byte("Hello, World!")
	result := ParseBody(raw, "text/plain")

	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", s)
	}
}

func TestParseBody_XML(t *testing.T) {
	raw := []byte(`<request><method>doSomething</method></request>`)
	result := ParseBody(raw, "application/xml")

	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string for XML body, got %T", result)
	}
	if s != string(raw) {
		t.Errorf("expected XML preserved as string")
	}
}

func TestParseBody_FormURLEncoded(t *testing.T) {
	raw := []byte("username=alice&password=secret")
	result := ParseBody(raw, "application/x-www-form-urlencoded")

	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string for form body, got %T", result)
	}
	if s != "username=alice&password=secret" {
		t.Errorf("unexpected form body: %q", s)
	}
}

func TestParseBody_Empty(t *testing.T) {
	result := ParseBody(nil, "application/json")
	if result != nil {
		t.Errorf("expected nil for empty body, got %v", result)
	}
}

func TestDecodeBody_JSON(t *testing.T) {
	body := map[string]any{"name": "Alice"}
	data, err := DecodeBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"name":"Alice"}` {
		t.Errorf("unexpected JSON output: %s", string(data))
	}
}

func TestDecodeBody_Base64Map(t *testing.T) {
	raw := []byte{0x08, 0x96, 0x01}
	encoded := base64.StdEncoding.EncodeToString(raw)

	body := map[string]any{
		"data":     encoded,
		"encoding": "base64",
	}

	data, err := DecodeBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != len(raw) {
		t.Errorf("decoded length mismatch: %d vs %d", len(data), len(raw))
	}
	for i := range raw {
		if data[i] != raw[i] {
			t.Errorf("byte %d mismatch: %02x vs %02x", i, data[i], raw[i])
		}
	}
}

func TestDecodeBody_EncodedBodyStruct(t *testing.T) {
	raw := []byte{0x01, 0x02, 0x03}
	eb := &EncodedBody{
		Data:     base64.StdEncoding.EncodeToString(raw),
		Encoding: BodyEncodingBase64,
	}

	data, err := DecodeBody(eb)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 3 || data[0] != 0x01 {
		t.Errorf("unexpected decoded data: %v", data)
	}
}

func TestDecodeBody_Nil(t *testing.T) {
	data, err := DecodeBody(nil)
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Errorf("expected nil for nil body")
	}
}

func TestRoundTrip_BinaryBody(t *testing.T) {
	// Simulate a full round-trip: record binary -> store -> load -> replay
	raw := []byte{0x08, 0x96, 0x01, 0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}

	// Record: parse body
	parsed := ParseBody(raw, "application/protobuf")

	// Store: the parsed body goes into a snapshot which gets marshaled to JSON
	// Load: it comes back as a map
	eb := parsed.(*EncodedBody)
	asMap := map[string]any{
		"data":     eb.Data,
		"encoding": eb.Encoding,
	}

	// Replay: decode body back to bytes
	decoded, err := DecodeBody(asMap)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != len(raw) {
		t.Fatalf("round-trip length mismatch: %d vs %d", len(decoded), len(raw))
	}
	for i := range raw {
		if decoded[i] != raw[i] {
			t.Errorf("round-trip byte %d mismatch: %02x vs %02x", i, decoded[i], raw[i])
		}
	}
}
