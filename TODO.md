# TODO: Missing Specification Features

This document tracks features from the specification that are not yet fully implemented.

## Critical Features (Spec Non-Compliance)

### 1. Outgoing Request Recording and Replay

**Status:** ❌ NOT IMPLEMENTED

**Specification Requirement:**
> When the service makes outgoing HTTP calls (e.g., to third-party APIs):
> - Record mode: Captures outgoing requests and their responses.
> - Replay mode: Intercepts outgoing calls and returns the recorded responses (acts as a mock server), ensuring tests are fully isolated.

**Current State:**
- The `OutgoingRequests` field exists in the snapshot structure
- The mock server exists and can replay outgoing requests
- **BUT:** The recorder does NOT capture outgoing requests during recording
- The snapshot's `OutgoingRequests` field is always empty

**Why This Is Critical:**
- Tests cannot verify outgoing API calls made by the service
- Replay cannot properly mock external dependencies
- Services that depend on external APIs cannot be fully tested in isolation

**Implementation Requirements:**
1. **Recording Phase:**
   - Intercept HTTP client calls made by the service
   - Options:
     - Instrument Go's `net/http` transport layer
     - Provide a custom `http.Client` wrapper
     - Use a transparent proxy for all outgoing traffic
   - Capture: method, URL, headers, body
   - Capture: response status, headers, body
   - Store in snapshot's `OutgoingRequests` array

2. **Replay Phase:**
   - Mock server URL needs to be injected into service configuration
   - Options:
     - Environment variable: `HTTP_PROXY=http://localhost:{mock_port}`
     - Service-specific config override
     - DNS hijacking (advanced)
   - Service must route outgoing calls through the mock server

**Estimated Effort:** HIGH (requires architecture changes)

**Workaround:**
- Manually mock external dependencies at the service level
- Use environment-specific configuration to point to mock servers

---

### 2. Mock Server URL Injection

**Status:** ❌ PARTIALLY IMPLEMENTED

**Specification Requirement:**
> The mock server address needs to be communicated to the service so outgoing requests are routed through it.

**Current State:**
- Mock server starts successfully and returns its address
- Address is stored in a variable but never used
- Service has no way to know about the mock server

**Implementation Requirements:**
1. Add configuration option for how to inject the mock URL:
   ```yaml
   replay:
     mock_injection:
       method: "env"  # env | config | dns
       env_var: "HTTP_PROXY"
   ```

2. Before firing request in replay mode:
   - Set environment variables for the service process
   - OR rewrite service config files
   - OR use iptables/hosts file manipulation

3. Alternative: Document that users must configure their service to use the mock server URL

**Estimated Effort:** MEDIUM

**Workaround:**
- Manually configure service to use mock server URL
- Use service-specific environment variables

---

### 3. Parallel Replay Execution

**Status:** ❌ NOT IMPLEMENTED

**Specification Requirement:**
> Configuration includes `replay.parallel: true` for concurrent snapshot execution.

**Current State:**
- Config field exists but is not used
- `ReplayAll()` executes snapshots sequentially

**Implementation Requirements:**
```go
func (r *Replayer) ReplayAll(snapshots []*snapshot.Snapshot, paths []string) []TestResult {
    if r.config.Replay.Parallel {
        // Use goroutines + sync.WaitGroup
        // Need to ensure database isolation per goroutine
        // OR serialize database access with mutex
    }
    // Current sequential implementation
}
```

**Challenges:**
- Database state restoration is not thread-safe
- Each snapshot modifies global DB state
- Would need separate database instances per goroutine OR serialization

**Estimated Effort:** MEDIUM

**Recommendation:**
- Parallel execution requires database-per-snapshot or careful locking
- May provide limited benefit due to database contention
- Consider removing from spec or documenting as "future enhancement"

---

## Important Missing Features

### 4. Environment Variable Support for Configuration

**Status:** ❌ NOT IMPLEMENTED

**Specification Requirement:**
> Support for environment variables in configuration files to avoid storing credentials in plaintext.

**Current State:**
- Config values are read directly from YAML
- No environment variable substitution

**Implementation Requirements:**
```yaml
database:
  connection_string: "${DB_CONNECTION_STRING}"  # Should expand from env var
```

Add function to expand environment variables:
```go
func expandEnvVars(cfg *Config) {
    cfg.Database.ConnectionString = os.ExpandEnv(cfg.Database.ConnectionString)
    // ... expand other fields
}
```

**Estimated Effort:** LOW

**Priority:** HIGH (security improvement)

---

### 5. Field-Level Redaction During Recording

**Status:** ❌ NOT IMPLEMENTED

**Specification Requirement:**
> Ability to redact sensitive fields during snapshot creation (not just during assertion).

**Current State:**
- `ignore_fields` only used during replay/assertion
- Sensitive data is still captured in snapshots

**Implementation Requirements:**
```yaml
recording:
  redact_fields:
    - "request.headers.Authorization"
    - "*.password"
    - "*.ssn"
```

Replace redacted values with `"[REDACTED]"` during snapshot creation.

**Estimated Effort:** MEDIUM

**Priority:** HIGH (security/privacy)

---

### 6. Rate Limiting on Recording Proxy

**Status:** ❌ NOT IMPLEMENTED

**Specification Requirement:**
> Protection against DoS attacks and resource exhaustion.

**Current State:**
- Proxy accepts unlimited requests
- No rate limiting or connection limits

**Implementation Requirements:**
```yaml
recording:
  rate_limit:
    requests_per_second: 100
    max_concurrent: 50
```

Use `golang.org/x/time/rate` for rate limiting.

**Estimated Effort:** LOW

**Priority:** MEDIUM (security)

---

### 7. Authentication for Recording Proxy

**Status:** ❌ NOT IMPLEMENTED

**Specification Requirement:**
> Basic authentication, API keys, or TLS support for the recorder.

**Current State:**
- Proxy has no authentication
- Anyone with network access can record snapshots

**Implementation Requirements:**
```yaml
recording:
  auth:
    type: "api_key"  # none | api_key | basic | mtls
    api_key: "${RECORDER_API_KEY}"
  tls:
    enabled: true
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
```

**Estimated Effort:** MEDIUM

**Priority:** HIGH (security)

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

**Status:** ❌ NOT IMPLEMENTED

**Current State:**
- Uses `log.Printf()` throughout
- No log levels (DEBUG, INFO, WARN, ERROR)
- No structured logging

**Recommendation:**
Use a structured logging library like `go.uber.org/zap` or `github.com/rs/zerolog`.

**Estimated Effort:** LOW

---

### 10. Extract Duplicated Request Firing Logic

**Status:** ❌ NOT FIXED

**Current State:**
- Request firing logic duplicated in:
  - `internal/replayer/replayer.go` (lines 140-192)
  - `internal/cli/helpers.go` (lines 19-71)

**Implementation:**
Create `internal/http/client.go`:
```go
func FireRequest(baseURL string, req snapshot.Request, timeout time.Duration) (*snapshot.Response, error) {
    // Shared implementation
}
```

**Estimated Effort:** LOW

**Priority:** MEDIUM (code quality)

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

**Status:** ❌ INCOMPLETE

**Not Tested:**
- `internal/cli/helpers.go` - no tests
- `internal/recorder/recorder.go` - no tests
- `internal/replayer/replayer.go` - no tests
- Error scenarios (DB connection failures, corrupt snapshots, network timeouts)
- Security scenarios (path traversal, SQL injection attempts)
- Integration tests (end-to-end workflows)

**Estimated Effort:** HIGH

**Priority:** MEDIUM

---

## Summary

| Feature | Status | Priority | Effort |
|---------|--------|----------|--------|
| Outgoing request recording | ❌ Not implemented | CRITICAL | HIGH |
| Mock server URL injection | ❌ Partial | CRITICAL | MEDIUM |
| Parallel replay | ❌ Not implemented | LOW | MEDIUM |
| Environment variable support | ❌ Not implemented | HIGH | LOW |
| Field-level redaction | ❌ Not implemented | HIGH | MEDIUM |
| Rate limiting | ❌ Not implemented | MEDIUM | LOW |
| Proxy authentication | ❌ Not implemented | HIGH | MEDIUM |
| MongoDB/Redis support | ❌ Not implemented | MEDIUM | HIGH |
| Structured logging | ❌ Not implemented | LOW | LOW |
| Deduplicate request firing | ❌ Not fixed | MEDIUM | LOW |
| Constants for magic strings | ❌ Not fixed | LOW | LOW |
| Test coverage | ❌ Incomplete | MEDIUM | HIGH |

**Recommendation:**
1. **Immediate Priority:** Environment variable support, field-level redaction, proxy authentication
2. **Medium Priority:** Outgoing request recording (requires architecture redesign)
3. **Long-term:** MongoDB/Redis support, parallel replay, comprehensive test coverage
