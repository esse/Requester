# TODO: Missing Specification Features

This document tracks features from the specification that are not yet fully implemented.

## Critical Features (Spec Non-Compliance)

### 1. Outgoing Request Recording and Replay

**Status:** ✅ IMPLEMENTED

**Specification Requirement:**
> When the service makes outgoing HTTP calls (e.g., to third-party APIs):
> - Record mode: Captures outgoing requests and their responses.
> - Replay mode: Intercepts outgoing calls and returns the recorded responses (acts as a mock server), ensuring tests are fully isolated.

**Implementation:**
- `internal/recorder/outgoing.go` implements a forward HTTP proxy (`OutgoingProxy`) that captures all outgoing requests
- During recording, the proxy is started automatically and captures method, URL, headers, body, and response
- The recorder drains captured requests after each incoming request and stores them in the snapshot's `OutgoingRequests` field
- During replay, the mock server (`internal/mock/server.go`) replays recorded responses

---

### 2. Mock Server URL Injection

**Status:** ✅ IMPLEMENTED

**Specification Requirement:**
> The mock server address needs to be communicated to the service so outgoing requests are routed through it.

**Implementation:**
- The `service.mock_env_var` config field (default: `SNAPSHOT_MOCK_URL`) controls the environment variable name
- During replay, if outgoing requests exist, a mock server is started
- The mock URL is injected via the configured environment variable when starting the service subprocess
- `internal/replayer/service.go` handles subprocess lifecycle with environment injection

---

### 3. Parallel Replay Execution

**Status:** ✅ IMPLEMENTED

**Specification Requirement:**
> Configuration includes `replay.parallel: true` for concurrent snapshot execution.

**Implementation:**
- `ReplayAll()` in `internal/replayer/replayer.go` checks `config.Replay.Parallel`
- When enabled, snapshots are replayed concurrently using goroutines and `sync.WaitGroup`
- Results are stored in a pre-allocated slice indexed by position (thread-safe)

**Usage:**
```yaml
replay:
  parallel: true
```

**Note:** Parallel execution requires database isolation per snapshot (separate test databases or serialized DB access) to avoid state conflicts.

---

## Important Missing Features

### 4. Environment Variable Support for Configuration

**Status:** ✅ IMPLEMENTED

**Specification Requirement:**
> Support for environment variables in configuration files to avoid storing credentials in plaintext.

**Implementation:**
- Config values support environment variable expansion using `${VAR_NAME}` syntax
- Implemented in `config.go` via `expandEnvVars()` function using `os.ExpandEnv`
- Works for all string configuration fields including database connection strings

**Usage Example:**
```yaml
database:
  connection_string: "${DB_CONNECTION_STRING}"  # Expands from env var
```

---

### 5. Field-Level Redaction During Recording

**Status:** ✅ IMPLEMENTED

**Specification Requirement:**
> Ability to redact sensitive fields during snapshot creation (not just during assertion).

**Implementation:**
- `recording.redact_fields` config field specifies paths to redact
- Redaction runs in `buildSnapshot()` before saving to disk
- Supports structured paths: `request.headers.Authorization`, `response.body.password`
- Supports wildcard paths: `*.password` redacts at any depth in request/response bodies and outgoing requests
- Redacted values are replaced with `[REDACTED]`

**Usage:**
```yaml
recording:
  redact_fields:
    - "request.headers.Authorization"
    - "*.password"
    - "*.ssn"
    - "response.body.api_key"
```

---

### 6. Rate Limiting on Recording Proxy

**Status:** ✅ IMPLEMENTED

**Specification Requirement:**
> Protection against DoS attacks and resource exhaustion.

**Implementation:**
- `recording.rate_limit` config section with `requests_per_second` and `max_concurrent`
- Uses `golang.org/x/time/rate` for token-bucket rate limiting
- Concurrency limiting via channel-based semaphore
- Returns HTTP 429 (Too Many Requests) when rate exceeded
- Returns HTTP 503 (Service Unavailable) when concurrency exceeded

**Usage:**
```yaml
recording:
  rate_limit:
    requests_per_second: 100
    max_concurrent: 50
```

---

### 7. Authentication for Recording Proxy

**Status:** ✅ IMPLEMENTED

**Specification Requirement:**
> Basic authentication, API keys, or TLS support for the recorder.

**Implementation:**
- `recording.proxy_auth_token` config field enables Bearer token authentication
- Implemented as middleware in `recorder.go` via `withAuth()`
- Returns 401 with WWW-Authenticate header for missing auth
- Returns 403 for invalid tokens
- Auth header is stripped before proxying to prevent leaking to the service
- Supports environment variable expansion: `proxy_auth_token: "${RECORDER_API_KEY}"`

**Usage:**
```yaml
recording:
  proxy_auth_token: "${RECORDER_API_KEY}"
```

---

### 8. MongoDB and Redis Support

**Status:** ❌ NOT IMPLEMENTED

**Specification Note:**
> MongoDB and Redis are marked as "Planned (v2)" in the spec.

**Current State:**
- Only SQL databases supported (Postgres, MySQL, SQLite)

**Implementation Requirements:**
- New snapshotter implementations for MongoDB and Redis
- Different data serialization (BSON for Mongo, key-value for Redis)
- Different diff computation strategies

**Estimated Effort:** HIGH

**Priority:** MEDIUM (feature expansion)

---

## Code Quality Improvements

### 9. Structured Logging

**Status:** ✅ IMPLEMENTED

**Implementation:**
- All `log.Printf()` calls replaced with Go's built-in `log/slog` (no external dependency)
- Supports log levels: DEBUG, INFO, WARN, ERROR
- Structured key-value pairs for all log messages (e.g., `"method", "GET", "status", 200`)
- `--log-level` CLI flag to control log verbosity (default: info)
- `internal/logger/logger.go` provides `Setup()` to configure the default handler
- Each component uses contextual attributes (e.g., `"component", "mock"`, `"component", "outgoing_proxy"`)

---

### 10. Extract Duplicated Request Firing Logic

**Status:** ✅ FIXED

**Implementation:**
- Created `internal/httpclient/client.go` with shared `FireRequest()` function
- `internal/replayer/replayer.go` now delegates to `httpclient.FireRequest()`
- `internal/cli/helpers.go` now delegates to `httpclient.FireRequest()`
- Single implementation handles body decoding, request construction, timeout, and response parsing

---

### 11. Replace Magic Strings with Constants

**Status:** ❌ NOT FIXED

**Examples:**
- Database discovery queries hardcoded
- Content-type strings scattered throughout
- Field names ("id", "status", "body")

**Recommendation:**
```go
const (
    ContentTypeJSON = "application/json"
    ContentTypeProtobuf = "application/protobuf"
    // ...
)
```

**Estimated Effort:** LOW

**Priority:** LOW (code quality)

---

## Testing Gaps

### 12. Missing Test Coverage

**Status:** ✅ SUBSTANTIALLY ADDRESSED

**Tested:**
- `internal/httpclient/` - GET, POST, nil body scenarios
- `internal/recorder/` - auth middleware, outgoing proxy capture, redaction (7 cases), rate limiting
- `internal/replayer/replayer.go` - ReplayOne success/failure, response mismatch, DB errors, request errors, outgoing requests, parallel/sequential ReplayAll
- `internal/replayer/service.go` - empty command, short-lived command, extra env injection, nil stop, graceful stop
- `internal/cli/helpers.go` - fireRequestForUpdate, computeDiffForUpdate, newSnapshotterForUpdate error
- `internal/logger/` - all log levels, case insensitivity, unknown level fallback
- `internal/asserter/` - existing tests
- `internal/config/` - existing tests
- `internal/db/` - existing tests
- `internal/mock/` - existing tests
- `internal/reporter/` - existing tests
- `internal/security/` - existing tests
- `internal/snapshot/` - existing tests

**Still Not Tested:**
- CLI command integration tests (requires full cobra command execution)
- End-to-end integration tests (full record + replay workflow)

**Priority:** LOW (remaining gaps are integration-level)

---

## Summary

| Feature | Status | Priority | Effort |
|---------|--------|----------|--------|
| Outgoing request recording | ✅ Implemented | CRITICAL | - |
| Mock server URL injection | ✅ Implemented | CRITICAL | - |
| Parallel replay | ✅ Implemented | LOW | - |
| Environment variable support | ✅ Implemented | HIGH | - |
| Field-level redaction | ✅ Implemented | HIGH | - |
| Rate limiting | ✅ Implemented | MEDIUM | - |
| Proxy authentication | ✅ Implemented | HIGH | - |
| Deduplicate request firing | ✅ Fixed | MEDIUM | - |
| MongoDB/Redis support | ❌ Not implemented | MEDIUM | HIGH |
| Structured logging | ✅ Implemented | LOW | - |
| Constants for magic strings | ❌ Not fixed | LOW | LOW |
| Test coverage | ✅ Substantial | MEDIUM | - |

**Remaining Work:**
1. **Low Priority:** Constants for magic strings
2. **Long-term:** MongoDB/Redis support, CLI integration tests
