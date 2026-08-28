# 🔒 Security Architecture & Threat Model

This document outlines Flagura's security model, cryptographic guarantees, threat mitigations (STRIDE analysis), and defensive engineering practices.

---

## 🛡️ STRIDE Threat Model & Mitigations

| Threat Category (STRIDE) | Attack Vector | Architectural Mitigation in Flagura |
| :--- | :--- | :--- |
| **Spoofing** | Forged session tokens or identity spoofing | Cryptographically secure 256-bit entropy tokens (`crypto/rand`) stored in `HttpOnly`, `SameSite=Lax` cookies. |
| **Tampering** | Modifying flag rules or SQL injection | 100% Parameterized queries (`$1, $2`). Input payload length validation (`http.MaxBytesReader` 1MB limit). |
| **Repudiation** | Denying an unapproved flag toggle | Immutable audit logs recording actor ID, email, action type, timestamp, and state diff. |
| **Information Disclosure** | Credential theft or session interception | Salted Bcrypt password hashing (never stored or logged). HSTS (`Strict-Transport-Security`) enforcing TLS. |
| **Denial of Service (DoS)** | Massive payload injection or slow-loris | Sub-microsecond local evaluations (< 400ns) prevent CPU exhaustion. Request body capping (1MB). |
| **Elevation of Privilege** | Member user creating/deleting flags | Role-Based Access Control (RBAC) middleware verifying `admin` role on all mutating API endpoints. |

---

## 🔐 Cryptographic & Security Primitives

### 1. Password Hashing (Bcrypt)
- Algorithm: `bcrypt` (Blowfish-based adaptive hashing).
- Salt: Automatically generated with a minimum work factor cost of 10.
- Resistance: Highly resistant to offline rainbow table and GPU brute-force attacks.

### 2. Session Token Generation
```go
func generateSessionToken() (string, error) {
    bytes := make([]byte, 32) // 256 bits of cryptographically secure entropy
    if _, err := rand.Read(bytes); err != nil {
        return "", err
    }
    return hex.EncodeToString(bytes), nil // 64-character hex string
}
```

### 3. Dynamic Secure Cookie Policy
```go
http.SetCookie(w, &http.Cookie{
    Name:     SessionCookieName,
    Value:    token,
    Path:     "/",
    Expires:  time.Now().Add(7 * 24 * time.Hour), // 7-day TTL
    HttpOnly: true,                               // Prevents XSS cookie theft
    SameSite: http.SameSiteLaxMode,               // Prevents Cross-Site Request Forgery (CSRF)
    Secure:   isSecure,                           // Dynamic based on HTTPS / Production
})
```

---

## 🌐 HTTP Security Headers Baseline

Every request handled by Flagura (standalone server or Vercel Edge) is passed through `SecurityHeadersMiddleware`:

```http
Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.tailwindcss.com https://cdn.jsdelivr.net https://unpkg.com https://cdnjs.cloudflare.com; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data: https: blob:; connect-src 'self' https: wss: ws:;
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Frame-Options: SAMEORIGIN
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: camera=(), microphone=(), geolocation=()
```

---

## 🔍 Automated CI Security Pipeline

1. **`govulncheck ./...`**: Runs before every commit and in GitHub Actions CI to identify known CVEs in Go dependencies.
2. **`gosec ./...`**: Performs AST-based Static Application Security Testing (SAST) for code-level security issues.
3. **`gitleaks`**: Scans the git history and commits for accidental API key or secret leakage.
