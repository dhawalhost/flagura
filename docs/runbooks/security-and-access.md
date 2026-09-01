# 🛡️ Security, Access & Credential Management Runbook

This runbook provides step-by-step instructions for managing user roles, enforcing password policies, handling password resets, credential rotation, responding to security vulnerabilities, and maintaining compliance in Flagura.

---

## 1. User Provisioning & Role-Based Access Control (RBAC)

Flagura supports three distinct user roles:

| Role | Permissions | Mutation Access | Deletion & Reset Access | Governance Reviewer |
| :--- | :--- | :---: | :---: | :---: |
| `admin` | Full system control & emergency operations | ✅ Allowed | ✅ Allowed (`DELETE /flags`, `POST /reset`) | ✅ Auto-assigned for 4-eyes approvals |
| `developer` | Flag creation, rollout adjustments, toggle switches | ✅ Allowed | ❌ Restricted (`403 Forbidden`) | ❌ Author only |
| `viewer` | Read-only access to dashboard and logs | ❌ Restricted | ❌ Restricted | ❌ No approval rights |

### Provisioning a New User via API:
```bash
curl -X POST "https://flagura.yourdomain.com/api/v1/auth/signup" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Jane Doe",
    "email": "jane@company.com",
    "password": "<YOUR_STRONG_PASSWORD>",
    "role": "developer"
  }'
```

---

## 2. Password Security Policy & Validation

Flagura enforces **defense-in-depth password complexity** on all signups and password resets:

### Policy Rules:
1. **Length**: Minimum 8 characters.
2. **Uppercase**: At least one uppercase letter (`A-Z`).
3. **Lowercase**: At least one lowercase letter (`a-z`).
4. **Numeric**: At least one numeric digit (`0-9`).
5. **Special Symbol**: At least one symbol (`!@#$%^&*()_+-=[]{}|;:,.<>?`).

### Client & Server Enforcement:
- **Pre-Submission Interception**: Real-time client-side evaluation displays visual checkmarks (`✓`) and a strength meter. Form submission is blocked until all 5 criteria are met.
- **Backend Defense-in-Depth**: `validatePasswordComplexity()` in `pkg/api/handlers_auth.go` validates every password before hashing with bcrypt (cost factor: 10).

---

## 3. Password Reset & Recovery Flows

### Scenario A: Self-Service Reset (When SMTP is Configured)
1. User navigates to `/auth` and clicks **"Forgot password?"**.
2. Flagura generates a cryptographically secure, single-use token valid for **15 minutes**.
3. A branded, responsive HTML reset email is dispatched containing the secure link:
   ```
   https://flagura.yourdomain.com/auth?mode=reset&token=flg_rst_5ad60866b...
   ```
4. User clicks the link, inputs a compliant new password, and their session is updated.

### Scenario B: Self-Hosted Reset (When SMTP is Not Configured)
If `SMTP_HOST` is not configured, email delivery is disabled by default (`DisabledMailer`). The API returns:
```json
{
  "error": "Email Service Disabled",
  "message": "Email delivery is disabled on this instance (SMTP is not configured). Please contact your workspace administrator to reset your credentials."
}
```

An administrator can manually reset a user's password directly via PostgreSQL:

```bash
# 1. Generate a bcrypt hash for the new password (e.g. using htpasswd or Go):
# Using go run:
NEW_HASH=$(go run -e 'package main; import ("fmt"; "golang.org/x/crypto/bcrypt"); func main() { h, _ := bcrypt.GenerateFromPassword([]byte("NewStrongP@ssw0rd!"), 10); fmt.Println(string(h)) }')

# 2. Update user record in PostgreSQL
psql "$DATABASE_URL" -c "
  UPDATE users 
  SET password_hash = '$NEW_HASH', updated_at = NOW() 
  WHERE email = 'user@company.com';
"

# 3. Invalidate active sessions to force re-login
psql "$DATABASE_URL" -c "
  DELETE FROM sessions 
  WHERE user_id IN (SELECT id FROM users WHERE email = 'user@company.com');
"
```

---

## 4. Compromised Credentials & Session Revocation

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
-- Forces all users across the organization to re-authenticate immediately
TRUNCATE TABLE sessions;
```

---

## 5. Transactional Email & Governance Setup

Flagura uses dedicated HTML templates for transactional notifications ([`pkg/email/templates/`](../../pkg/email/templates)):
- `password_reset.html`: 15-minute recovery token link.
- `welcome.html`: Account provisioning and SDK quickstart snippets.
- `change_request.html`: 4-eyes approval review notification for production flag changes.

### Environment Configuration:
```bash
# Outbound SMTP credentials
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USERNAME=apikey
SMTP_PASSWORD=SG.your_api_key_here
SMTP_FROM=no-reply@yourcompany.com

# Branding and support customization
FLAGURA_BRAND_NAME="Acme Feature Flags"
FLAGURA_SUPPORT_EMAIL="devops@yourcompany.com"

# Optional: Specific governance reviewer list (defaults to all database admins if unset)
FLAGURA_GOVERNANCE_EMAILS="lead-architect@yourcompany.com,sec-ops@yourcompany.com"
```

---

## 6. Vulnerability Management & Automated Scanning

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

---

## 7. HTTP Security Headers Verification

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
