package snapshot

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// BodyEncoding indicates how a body was encoded in the snapshot.
const (
	BodyEncodingJSON   = ""       // default: stored as parsed JSON
	BodyEncodingText   = "text"   // stored as UTF-8 string
	BodyEncodingBase64 = "base64" // stored as base64 (for binary payloads like protobuf)
)

// EncodedBody wraps a body payload with its encoding metadata.
// For JSON bodies, Body is the parsed object and Encoding is empty.
// For text bodies, Body is a string and Encoding is "text".
// For binary bodies, Body is a base64 string and Encoding is "base64".
type EncodedBody struct {
	Data     any    `json:"data" yaml:"data"`
	Encoding string `json:"encoding,omitempty" yaml:"encoding,omitempty"`
}

// ParseBody interprets raw bytes based on content type.
// JSON content types are parsed into structured data.
// Text content types are stored as UTF-8 strings.
// Binary content (protobuf, msgpack, grpc, octet-stream) is base64-encoded.
func ParseBody(raw []byte, contentType string) any {
	if len(raw) == 0 {
		return nil
	}

	ct := strings.ToLower(contentType)

	// Binary content types: store as base64
	if isBinaryContentType(ct) {
		return &EncodedBody{
			Data:     base64.StdEncoding.EncodeToString(raw),
			Encoding: BodyEncodingBase64,
		}
	}

	// Try JSON parse first (works for application/json, application/json-rpc, etc.)
	if isJSONContentType(ct) || ct == "" {
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err == nil {
			return parsed
		}
	}

	// Fall back to string for text types, base64 for anything else
	if isTextContentType(ct) {
		return string(raw)
	}

	// Unknown type: try JSON, then text, then base64
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err == nil {
		return parsed
	}

	// Check if valid UTF-8 text
	s := string(raw)
	for _, r := range s {
		if r == '\ufffd' {
			// Contains replacement chars — likely binary
			return &EncodedBody{
				Data:     base64.StdEncoding.EncodeToString(raw),
				Encoding: BodyEncodingBase64,
			}
		}
	}
	return s
}

// DecodeBody reverses ParseBody, returning raw bytes suitable for HTTP transport.
func DecodeBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	// Check if it's an EncodedBody (could come back as map from JSON deserialization)
	if m, ok := body.(map[string]any); ok {
		if enc, hasEnc := m["encoding"]; hasEnc {
			data, _ := m["data"].(string)
			switch enc {
			case BodyEncodingBase64:
				return base64.StdEncoding.DecodeString(data)
			case BodyEncodingText:
				return []byte(data), nil
			}
		}
	}

	// Check native EncodedBody struct
	if eb, ok := body.(*EncodedBody); ok {
		data, _ := eb.Data.(string)
		switch eb.Encoding {
		case BodyEncodingBase64:
			return base64.StdEncoding.DecodeString(data)
		case BodyEncodingText:
			return []byte(data), nil
		}
	}

	// Default: marshal as JSON
	return json.Marshal(body)
}

func isBinaryContentType(ct string) bool {
	binaryTypes := []string{
		"application/grpc",
		"application/grpc-web",
		"application/grpc-web+proto",
		"application/protobuf",
		"application/x-protobuf",
		"application/x-google-protobuf",
		"application/msgpack",
		"application/x-msgpack",
		"application/octet-stream",
		"application/cbor",
		"application/thrift",
		"application/avro",
		"application/flatbuffers",
	}
	for _, bt := range binaryTypes {
		if strings.HasPrefix(ct, bt) {
			return true
		}
	}
	return false
}

func isJSONContentType(ct string) bool {
	return strings.Contains(ct, "json") || strings.Contains(ct, "json-rpc")
}

func isTextContentType(ct string) bool {
	return strings.HasPrefix(ct, "text/") ||
		strings.Contains(ct, "xml") ||
		strings.Contains(ct, "html") ||
		strings.Contains(ct, "form-urlencoded")
}
