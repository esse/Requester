package snapshot

// Content type constants for HTTP requests and responses.
const (
	ContentTypeJSON = "application/json"
)

// HTTP header name constants.
const (
	HeaderContentType     = "Content-Type"
	HeaderAuthorization   = "Authorization"
	HeaderWWWAuthenticate = "WWW-Authenticate"
)

// Snapshot file format identifiers.
const (
	FormatJSON = "json"
	FormatYAML = "yaml"
	FormatYML  = "yml"
)

// DefaultMockEnvVar is the default environment variable name used to inject the mock server URL.
const DefaultMockEnvVar = "SNAPSHOT_MOCK_URL"

// AuthSchemeBearer is the Bearer authentication scheme prefix.
const AuthSchemeBearer = "Bearer "
