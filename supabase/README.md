# Flagura Database & Cloud Deployment Guide

This guide covers setting up **Supabase PostgreSQL** for persistent storage and configuring **Vercel** with **GitHub Actions CI/CD** for zero-downtime serverless deployments.

---

## 1. Supabase Setup (Database Storage)

### Step 1: Create a Supabase Project
1. Go to [supabase.com](https://supabase.com) and create a new project.
2. Choose your preferred region and set a secure database password.

### Step 2: Apply the Database Schema
1. Open your Supabase project dashboard.
2. Navigate to **SQL Editor** in the left sidebar.
3. Paste the contents of [`schema.sql`](./schema.sql) and click **Run**.
4. This initializes:
   - `users` table (with bcrypt authentication hashes)
   - `sessions` table (with 7-day secure tokens)
   - `feature_flags` table (multi-environment JSONB configurations)
   - `audit_logs` table (timestamped immutable audit events)
   - `api_keys` table (environment-scoped SDK access keys)

### Step 3: Copy Connection String
1. Go to **Project Settings** $\rightarrow$ **Database**.
2. Scroll to **Connection string** $\rightarrow$ **Connection Pooling** (or URI).
3. Copy the URI string:
   ```bash
   postgres://postgres.[YOUR_PROJECT_REF]:[YOUR_PASSWORD]@aws-0-[REGION].pooler.supabase.com:6543/postgres?sslmode=require
   ```

---

## 2. Vercel Serverless Setup

Flagura runs natively as a Go serverless function on Vercel via [`api/index.go`](../api/index.go) and [`vercel.json`](../vercel.json).

### Step 1: Import Project in Vercel
1. Go to [vercel.com](https://vercel.com) $\rightarrow$ **Add New Project**.
2. Select your GitHub repository (`flagura`).
3. Set **Framework Preset** to `Other`.

### Step 2: Configure Environment Variables
Add the following variables in the Vercel project settings:

| Variable | Value | Description |
| :--- | :--- | :--- |
| `DATABASE_URL` | `postgres://postgres.[ref]:[pw]...` | Your Supabase connection URI |
| `ENVIRONMENT` | `production` | Deployment environment |

---

## 3. GitHub Actions CI/CD Setup

The repository includes two automated workflows in `.github/workflows/`:

* **`ci.yml`**: Runs on every Pull Request and Push to `main`.
  * Installs `templ` compiler
  * Generates templates and runs `go vet`
  * Runs full test suite with race detector and coverage analysis
  * Verifies binary compilation
* **`deploy.yml`**: Runs on push to `main` to build and deploy to Vercel production.

### Required GitHub Repository Secrets

In your GitHub repository, go to **Settings** $\rightarrow$ **Secrets and variables** $\rightarrow$ **Actions** $\rightarrow$ **New repository secret**:

1. **`VERCEL_TOKEN`**:
   - Generate in [Vercel Account Settings $\rightarrow$ Tokens](https://vercel.com/account/tokens).
2. **`VERCEL_ORG_ID`**:
   - Found in your Vercel team/account settings (or `.vercel/project.json` after running `vercel link`).
3. **`VERCEL_PROJECT_ID`**:
   - Found in your Vercel project settings $\rightarrow$ General.

---

## 4. Local Development

To run locally with Supabase:
```bash
# Set your Supabase connection string
export DATABASE_URL="postgres://postgres.[ref]:[pw]@..."

# Generate templates and run
templ generate
go run main.go
```
If `DATABASE_URL` is omitted, Flagura automatically runs with its zero-dependency in-memory edge store.
