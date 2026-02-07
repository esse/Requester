# Security Policy

## Security Overview

Service Snapshot Tester is designed to help with integration testing by recording and replaying service interactions. However, there are important security considerations when using this tool.

## Known Security Considerations

### 1. Sensitive Data in Snapshots

**Risk Level: HIGH**

Snapshots capture complete database state and request/response payloads. This may include:
- Personally Identifiable Information (PII)
- API keys and secrets
- Authentication tokens
- Passwords or password hashes

**Mitigations:**
- ⚠️ **Never commit snapshots containing production data to version control**
- Use test data only when recording snapshots
- Review snapshots manually before committing
- Use `.gitignore` to exclude snapshot directories if they contain sensitive data
- Consider encrypting snapshots at rest
- Use the `ignore_fields` configuration to exclude sensitive fields from snapshots

### 2. Database Credentials in Configuration Files

**Risk Level: HIGH**

Configuration files contain database connection strings with credentials in plaintext.

**Mitigations:**
- Use environment variables for sensitive configuration values
- Add config files with credentials to `.gitignore`
- Use separate test databases with limited privileges
- Never commit configuration files with production credentials
- Consider using a secrets management system (e.g., HashiCorp Vault, AWS Secrets Manager)

**Future Enhancement:**
- Support for environment variable substitution in config files (e.g., `connection_string: ${DB_CONNECTION_STRING}`)

### 3. Path Traversal Protection

**Risk Level: MEDIUM** ✅ Mitigated in current version

The tool validates all file paths to prevent directory traversal attacks:
- Config file paths are validated to prevent `..` sequences
- Snapshot file paths are validated to ensure they stay within the configured snapshot directory

### 4. No Authentication on Recording Proxy

**Risk Level: HIGH** ⚠️ Not yet addressed

The recording proxy binds to `0.0.0.0:port` by default with no authentication.

**Current State:**
- Anyone with network access can:
  - Record new snapshots
  - Trigger database snapshots (read operations)
  - Proxy requests through the service

**Recommended Mitigations:**
- Bind the proxy to `127.0.0.1` only (localhost)
- Use firewall rules to restrict access
- Run the proxy only in trusted environments (not production)
- Do not expose the proxy port publicly

**Future Enhancements:**
- API key authentication for the recording proxy
- TLS/HTTPS support
- IP allowlist configuration
- OAuth/JWT token validation

### 5. SQL Injection Protection

**Risk Level: LOW** ⚠️ Partially mitigated

Database queries use identifier quoting and parameterized values, but there's room for improvement.

**Current State:**
- Table and column names are properly quoted
- INSERT values use parameterized queries (protection against SQL injection)
- Discovery queries use fixed strings

**Best Practices Applied:**
- All user-controlled values use parameterized queries
- Identifiers are properly escaped/quoted based on database type

### 6. YAML Deserialization

**Risk Level: MEDIUM**

YAML parsing can be dangerous if the YAML library supports code execution directives.

**Current State:**
- Using `gopkg.in/yaml.v3` which does not support Ruby/Python object deserialization
- Snapshots and configs are parsed from user-controlled files

**Mitigations:**
- Use the latest version of the YAML library
- Validate snapshot file sources
- Only load snapshots from trusted sources

### 7. Replay Database Isolation

**Risk Level: MEDIUM**

The replay mode modifies database state by:
- Truncating tables (DELETE FROM)
- Inserting snapshot data
- Running the service's code which may perform additional operations

**Mitigations:**
- **Always use a separate test database for replay** (configured via `replay.test_database.connection_string`)
- Never point replay at production databases
- Use database users with limited permissions for testing
- Consider using containerized databases that can be reset easily

## Reporting Security Issues

If you discover a security vulnerability in this tool, please report it by:

1. **Do not** open a public GitHub issue
2. Email the maintainers directly at [security contact to be added]
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

We will respond within 48 hours and work with you to address the issue.

## Security Best Practices

When using Service Snapshot Tester:

### For Recording

1. **Use Test Data Only**
   - Never record snapshots against production databases
   - Create synthetic test data that doesn't contain real user information

2. **Isolate the Recording Environment**
   - Run the recording proxy only in development/staging environments
   - Use firewall rules to prevent unauthorized access
   - Bind to localhost (127.0.0.1) instead of 0.0.0.0

3. **Review Before Committing**
   - Manually review snapshots before committing to version control
   - Check for sensitive data in request/response bodies
   - Verify database state doesn't contain credentials or tokens

### For Replay

1. **Use Separate Test Databases**
   - Configure `replay.test_database.connection_string` to point to a test database
   - Never replay against production databases
   - Use containerized databases (Docker) for easy cleanup

2. **Limit Database Permissions**
   - Test database user should have minimal required permissions
   - Use read-only replicas where possible for snapshotting

3. **CI/CD Security**
   - Store database credentials in CI/CD secrets, not in code
   - Use ephemeral databases for CI test runs
   - Clean up test databases after CI runs complete

### General

1. **Keep Dependencies Updated**
   - Regularly update Go modules: `go get -u ./...`
   - Monitor security advisories for dependencies

2. **Access Control**
   - Limit access to snapshot files
   - Use file permissions to protect configuration files
   - Consider encryption at rest for sensitive snapshots

3. **Network Security**
   - Don't expose the recording proxy on public networks
   - Use VPNs or SSH tunnels if remote access is needed
   - Monitor network traffic during recording

## Compliance Considerations

If you're subject to compliance requirements (GDPR, HIPAA, PCI-DSS, SOC 2, etc.):

- **Data Minimization**: Only snapshot tables/fields necessary for testing
- **Data Anonymization**: Consider anonymizing or pseudonymizing data before recording
- **Retention Policies**: Implement cleanup policies for old snapshots
- **Audit Logging**: Track who creates and modifies snapshots
- **Encryption**: Encrypt snapshots at rest and in transit
- **Access Controls**: Implement role-based access to snapshots

## Security Checklist

Before using this tool in your environment:

- [ ] Reviewed this security policy
- [ ] Configured separate test databases for recording and replay
- [ ] Added config files with credentials to `.gitignore`
- [ ] Set up firewall rules to protect the recording proxy
- [ ] Reviewed snapshot content for sensitive data
- [ ] Implemented snapshot retention policies
- [ ] Configured ignore_fields for PII/sensitive data
- [ ] Tested with synthetic/anonymized data
- [ ] Documented security procedures for your team
- [ ] Set up monitoring/alerting for unauthorized access

## Version History

- **v1.0.0** (2026-02-07)
  - Added path traversal protection for config and snapshot files
  - Improved error handling in base64 decoding
  - Fixed SQL query parameterization
  - Initial security documentation

## Future Security Enhancements

Planned improvements:

- [ ] Environment variable support for configuration
- [ ] Authentication for recording proxy (API keys, OAuth)
- [ ] TLS/HTTPS support for recording proxy
- [ ] Field-level redaction/masking during recording
- [ ] PII detection and automatic redaction
- [ ] Snapshot encryption at rest
- [ ] Rate limiting for recording proxy
- [ ] Audit logging for snapshot operations
- [ ] Digital signatures for snapshot integrity verification
