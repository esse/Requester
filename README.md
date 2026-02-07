# Service Snapshot Tester

A tool for recording and replaying service interactions to enable deterministic integration testing. It captures the full lifecycle of requests — including database state before and after, incoming requests, and outgoing responses — and uses these "snapshots" to verify that services behave consistently over time.

## Features

- **Zero-effort test creation**: Tests are generated automatically by recording real interactions
- **Full-state verification**: Validates both response correctness and database side-effects
- **Regression detection**: Catches any change in response or database behavior immediately
- **Environment independence**: Tests run against a clean database without external dependencies
- **Multi-database support**: PostgreSQL, MySQL, and SQLite
- **Flexible output formats**: Text, JUnit XML, TAP, and JSON for CI/CD integration

## Installation

```bash
go install github.com/esse/snapshot-tester/cmd/snapshot-tester@latest
```

Or build from source:

```bash
git clone https://github.com/esse/Requester.git
cd Requester
go build -o snapshot-tester ./cmd/snapshot-tester
```

## Quick Start

### 1. Create a Configuration File

Create `snapshot-tester.yml`:

```yaml
service:
  name: "my-api"
  base_url: "http://localhost:3000"

database:
  type: "postgres"  # postgres | mysql | sqlite
  connection_string: "postgres://user:pass@localhost:5432/mydb"
  tables:           # Leave empty to snapshot all tables
    - users
    - orders

recording:
  proxy_port: 8080
  snapshot_dir: "./snapshots"
  format: "json"    # json | yaml
  ignore_headers:
    - "Authorization"
    - "X-Request-Id"
    - "Date"
  ignore_fields:    # Fields to ignore during comparison
    - "*.created_at"
    - "*.updated_at"
    - "response.headers.Date"

replay:
  test_database:
    connection_string: "postgres://user:pass@localhost:5432/mydb_test"
  strict_mode: true
  timeout_ms: 5000
```

### 2. Start Recording

Start your service, then start the recording proxy:

```bash
snapshot-tester record --config snapshot-tester.yml --tag happy-path
```

The proxy will listen on port 8080 (or the configured `proxy_port`) and forward requests to your service at `base_url`.

### 3. Make API Requests

Point your client to the proxy:

```bash
curl http://localhost:8080/users -X POST \
  -H "Content-Type: application/json" \
  -d '{"name": "Alice", "email": "alice@example.com"}'
```

Snapshots are automatically saved to `./snapshots/`.

### 4. Replay Snapshots

Run the recorded snapshots against your service:

```bash
snapshot-tester replay --config snapshot-tester.yml
```

This will:
1. Restore the database to the "before" state
2. Fire the recorded request
3. Compare the response and final database state
4. Report any differences

## Commands

### Record

Start the recording proxy to capture snapshots:

```bash
snapshot-tester record --config snapshot-tester.yml [--tag tag1,tag2]
```

### Replay

Replay all snapshots:

```bash
snapshot-tester replay --config snapshot-tester.yml
```

Replay a specific snapshot:

```bash
snapshot-tester replay --snapshot ./snapshots/my-api/POST_users/001.snapshot.json
```

Replay by tag:

```bash
snapshot-tester replay --tag happy-path
```

CI-friendly output:

```bash
snapshot-tester replay --ci  # Outputs JUnit XML
snapshot-tester replay --format tap
snapshot-tester replay --format json
```

### List

List all recorded snapshots:

```bash
snapshot-tester list --config snapshot-tester.yml
```

### Diff

Show the difference between expected and actual behavior for a specific snapshot:

```bash
snapshot-tester diff --snapshot ./snapshots/my-api/POST_users/001.snapshot.json
```

### Update

Update a snapshot with current behavior (accept new baseline):

```bash
snapshot-tester update --snapshot ./snapshots/my-api/POST_users/001.snapshot.json
```

## Snapshot File Format

Snapshots are stored as JSON or YAML files:

```json
{
  "id": "abc123",
  "timestamp": "2026-02-07T14:30:00Z",
  "service": "my-api",
  "tags": ["users", "happy-path"],
  
  "db_state_before": {
    "users": [
      {"id": 1, "name": "Alice", "email": "alice@example.com"}
    ]
  },
  
  "request": {
    "method": "POST",
    "url": "/users",
    "headers": {"Content-Type": "application/json"},
    "body": {"name": "Bob", "email": "bob@example.com"}
  },
  
  "response": {
    "status": 201,
    "headers": {"Content-Type": "application/json"},
    "body": {"id": 2, "name": "Bob", "email": "bob@example.com"}
  },
  
  "db_state_after": {
    "users": [
      {"id": 1, "name": "Alice", "email": "alice@example.com"},
      {"id": 2, "name": "Bob", "email": "bob@example.com"}
    ]
  },
  
  "db_diff": {
    "users": {
      "added": [{"id": 2, "name": "Bob", "email": "bob@example.com"}],
      "removed": [],
      "modified": []
    }
  }
}
```

## Dynamic Value Matching

Snapshots support dynamic matchers for values that change on each run:

- `"__ANY__"`: Matches any value
- `"__UUID__"`: Matches any valid UUID
- `"__ISO_DATE__"`: Matches any ISO 8601 timestamp

Example:

```json
{
  "response": {
    "body": {
      "id": "__UUID__",
      "created_at": "__ISO_DATE__",
      "name": "Bob"
    }
  }
}
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Snapshot Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_DB: mydb_test
          POSTGRES_PASSWORD: test
        ports:
          - 5432:5432

    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      
      - name: Install snapshot-tester
        run: go install github.com/esse/snapshot-tester/cmd/snapshot-tester@latest
      
      - name: Start service
        run: |
          go run ./cmd/my-service &
          sleep 2
      
      - name: Run snapshot tests
        run: snapshot-tester replay --config snapshot-tester.yml --ci
```

## Security Considerations

### Path Validation

All file paths (config files, snapshot files) are validated to prevent directory traversal attacks. The tool will reject paths containing `..` sequences that attempt to escape designated directories.

### Database Credentials

**Warning**: Database connection strings in config files contain credentials in plaintext. Best practices:

- Store credentials in environment variables and reference them in your config
- Use `.gitignore` to exclude config files with credentials
- Use separate databases for recording and testing
- Never commit snapshots containing sensitive data (PII, API keys, secrets)

### Sensitive Data in Snapshots

Snapshots capture the full database state and request/response bodies. To avoid exposing sensitive data:

1. Use `ignore_fields` to exclude sensitive fields from comparisons
2. Manually review snapshots before committing to version control
3. Consider encrypting snapshots at rest
4. Use test data instead of production data when recording

## Supported Databases

| Database   | Status   |
|------------|----------|
| PostgreSQL | ✅ Supported |
| MySQL      | ✅ Supported |
| SQLite     | ✅ Supported |
| MongoDB    | 🚧 Planned |
| Redis      | 🚧 Planned |

## Troubleshooting

### Snapshots fail with "DB state mismatch"

- Check if your service generates random IDs or timestamps
- Use dynamic matchers (`__UUID__`, `__ISO_DATE__`) for dynamic fields
- Add fields to `ignore_fields` in your config

### "Failed to snapshot database" error

- Verify database connection string is correct
- Ensure the database user has read/write permissions
- Check that all configured tables exist

### Proxy not receiving requests

- Verify the proxy port is correct
- Ensure your client is pointing to `http://localhost:{proxy_port}`
- Check that the service `base_url` is accessible from the proxy

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Submit a pull request

## License

[License information to be added]

## Authors

Built by the esse team.
