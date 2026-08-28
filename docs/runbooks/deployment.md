# 🚀 Deployment & Release Management Runbook

This runbook covers procedures for building, deploying, verifying, and rolling back Flagura across standalone Linux servers, Docker containers, and Vercel serverless environments.

---

## 1. Environment Variables Reference

| Variable | Description | Example / Default | Required |
| :--- | :--- | :--- | :---: |
| `PORT` | HTTP Server Port | `3000` | Optional (default: 3000) |
| `ENVIRONMENT` | Deployment Environment (`production`, `staging`, `development`) | `production` | **Required in Prod** |
| `DATABASE_URL` | Supabase / PostgreSQL connection string with SSL | `postgres://postgres.[REF]:[PW]@aws-0-[REGION].pooler.supabase.com:6543/postgres?sslmode=require` | Recommended |
| `APP_URL` | Canonical application URL (for CORS and cookies) | `https://flagura.yourdomain.com` | Optional |
| `SECURE_COOKIE`| Explicitly enforce `Secure: true` on cookies | `true` | Optional |

> [!IMPORTANT]
> When using Supabase PostgreSQL on serverless platforms (Vercel, AWS Lambda), always use the **Transaction Connection Pooler** on port **`6543`** to prevent PostgreSQL client connection exhaustion.

---

## 2. Deployment Targets

### Target A: Vercel Serverless (Recommended for Edge)

Flagura includes a native serverless entrypoint in [`api/index.go`](../../api/index.go) and rewrite rules in [`vercel.json`](../../vercel.json).

> [!TIP]
> For a full walkthrough on creating Supabase and Vercel accounts, setting up repository secrets, and configuring custom domains, refer to the **[Supabase & Vercel Integration Guide](../integrations/supabase-vercel-setup.md)**.

#### Automated GitHub Actions Workflow
Deployments are automatically triggered on push to `main` branch via [`.github/workflows/deploy.yml`](../../.github/workflows/deploy.yml).

#### Manual CLI Deployment:
```bash
# 1. Install Templ compiler & generate templates
go install github.com/a-h/templ/cmd/templ@latest
templ generate

# 2. Pull environment credentials & deploy
npx vercel pull --yes --environment=production --token=$VERCEL_TOKEN
npx vercel build --prod --token=$VERCEL_TOKEN
npx vercel deploy --prebuilt --prod --token=$VERCEL_TOKEN
```

---

### Target B: Docker Container Deployment

#### 1. Build Multi-Stage Production Image:
```bash
# Generate templates first
templ generate

# Build Docker Image
docker build -t flagura:latest -f - . << 'EOF'
FROM golang:1.26-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/flagura main.go

FROM alpine:3.19
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/flagura /app/flagura
EXPOSE 3000
ENTRYPOINT ["/app/flagura"]
EOF
```

#### 2. Run Container:
```bash
docker run -d \
  --name flagura-prod \
  --restart unless-stopped \
  -p 3000:3000 \
  -e ENVIRONMENT=production \
  -e DATABASE_URL="postgres://postgres.[REF]:[PW]@aws-0-[REGION].pooler.supabase.com:6543/postgres?sslmode=require" \
  flagura:latest
```

---

### Target C: Linux Systemd Standalone Service

#### 1. Compile Binary:
```bash
templ generate
CGO_ENABLED=0 go build -ldflags="-w -s" -o /usr/local/bin/flagura main.go
```

#### 2. Configure Systemd Service (`/etc/systemd/system/flagura.service`):
```ini
[Unit]
Description=Flagura Feature Flag Engine
After=network.target

[Service]
Type=simple
User=www-data
Group=www-data
WorkingDirectory=/var/www/flagura
Environment=PORT=3000
Environment=ENVIRONMENT=production
Environment=DATABASE_URL="postgres://postgres.[REF]:[PW]@aws-0-[REGION].pooler.supabase.com:6543/postgres?sslmode=require"
ExecStart=/usr/local/bin/flagura
Restart=always
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

#### 3. Enable and Start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable flagura
sudo systemctl restart flagura
sudo systemctl status flagura
```

---

## 3. Post-Deployment Smoke Test Protocol

Run these verification commands against your deployed URL:

```bash
TARGET_URL="https://flagura.yourdomain.com"

# 1. Verify Health Check
curl -s "${TARGET_URL}/api/health" | jq .

# Expected Output:
# {
#   "status": "ok",
#   "service": "flagura-engine",
#   "engine": "Flagura-FastPath-Deterministic",
#   "driver": "Supabase PostgreSQL"
# }

# 2. Verify Security Headers
curl -s -I "${TARGET_URL}/api/health" | grep -iE "x-content-type-options|x-frame-options|strict-transport-security|content-security-policy"

# 3. Verify Anonymous SDK Evaluation
curl -s -X POST "${TARGET_URL}/api/v1/evaluate" \
  -H "Content-Type: application/json" \
  -d '{"flags": ["ai-smart-search"], "context": {"user_id": "smoke-test-user-1", "environment": "production"}}' | jq .
```

---

## 4. Rollback Procedures

### Vercel Instant Rollback
1. Open Vercel Dashboard -> Project -> **Deployments**.
2. Select previous stable deployment.
3. Click **Instant Rollback** to promote it to Production in < 5 seconds.

### Docker / Systemd Rollback
```bash
# For Docker:
docker stop flagura-prod && docker rm flagura-prod
docker run -d --name flagura-prod -p 3000:3000 [PREVIOUS_IMAGE_TAG]

# For Systemd:
sudo cp /usr/local/bin/flagura.bak /usr/local/bin/flagura
sudo systemctl restart flagura
```
