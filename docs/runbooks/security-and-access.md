# 🛡️ Security, Access & Credential Management Runbook

This runbook provides step-by-step instructions for managing user roles, handling credential rotation, responding to security vulnerabilities, and maintaining compliance in Flagura.

---

## 1. User Provisioning & Role-Based Access Control (RBAC)

Flagura supports three distinct user roles:

| Role | Permissions | Mutation Access | Deletion & Reset Access |
| :--- | :--- | :---: | :---: |
| `admin` | Full system control | ✅ Allowed | ✅ Allowed (`DELETE /flags`, `POST /reset`) |
| `developer` | Flag creation, rollout adjustments, toggle switches | ✅ Allowed | ❌ Restricted (`403 Forbidden`) |
| `viewer` | Read-only access to dashboard and logs | ❌ Restricted | ❌ Restricted |

### Provisioning a New User:
```bash
curl -X POST "https://flagura.yourdomain.com/api/v1/auth/signup" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Jane Doe",
    "email": "jane@company.com",
    "password": "SecurePassword123!",
    "role": "developer"
  }'
```

---

## 2. Compromised Credentials & Session Revocation

If an employee leaves or administrator credentials are suspected of being leaked:

### Step 1: Immediate Session Revocation
Invalidate all active sessions for that user directly in PostgreSQL:
```sql
-- Revoke all active sessions for a specific user email
DELETE FROM sessions 
WHERE user_id IN (SELECT id FROM users WHERE email = 'compromised@company.com');
```

### Step 2: Invalidate ALL Active Sessions (Emergency Full Wipe)
```sql
-- Forces all users across the organization to re-authenticate
TRUNCATE TABLE sessions;
```

### Step 3: Rotate User Password
Generate a new bcrypt hash and update the user record:
```sql
-- Update password hash directly in PostgreSQL if needed
UPDATE users 
SET password_hash = '$2a$10$YourNewBcryptHashedPasswordHere...', updated_at = NOW() 
WHERE email = 'compromised@company.com';
```

---

## 3. Vulnerability Management & Automated Scanning

Flagura includes built-in automated security scanners in CI/CD.

### Running Local Security Audits:

```bash
# 1. Run SAST Security Scanner (Gosec)
go run github.com/securego/gosec/v2/cmd/gosec@latest -exclude-dir=web/views ./...

# 2. Run Dependency CVE Scanner (Govulncheck)
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# 3. Run Unit & Security Test Suite with Race Detector
go test -v -race ./...
```

### Triaging Dependabot Security Alerts
1. Review pull requests created by Dependabot in GitHub: `https://github.com/dhawalhost/flagura/pulls`.
2. Inspect the CVE advisory and bump compatibility.
3. Validate that CI tests pass before merging.

---

## 4. HTTP Security Headers Verification

Verify that your production reverse proxy (Cloudflare / Vercel / Nginx) and Flagura pass standard security header scans:

```bash
curl -s -I "https://flagura.yourdomain.com/api/health" | grep -iE \
  "x-content-type-options|x-frame-options|x-xss-protection|referrer-policy|strict-transport-security|content-security-policy"
```

Expected Output:
```http
X-Content-Type-Options: nosniff
X-Frame-Options: SAMEORIGIN
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
Strict-Transport-Security: max-age=31536000; includeSubDomains
Content-Security-Policy: default-src 'self'; script-src 'self' ...
```
