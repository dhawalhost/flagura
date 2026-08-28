# 🌐 Supabase & Vercel Production Integration Guide

This guide provides step-by-step instructions for provisioning **Supabase PostgreSQL** and **Vercel Serverless Functions** to host and maintain Flagura with zero maintenance, automatic scaling, and continuous deployment.

---

## ⚡ Guide Structure & Key Highlights

| Section | Focus Area | Key Highlights & Takeaways |
| :--- | :--- | :--- |
| **[1. Supabase Cloud Setup](#1-supabase-setup-persistent-database)** | Database & Storage | Project creation, table/index initialization via `schema.sql`, and **Transaction Pooler (Port `6543`)** configuration. |
| **[2. Vercel Serverless Setup](#2-vercel-setup-serverless-application)** | Edge Compute & Hosting | Zero-config Go serverless runtime via `vercel.json` and `api/index.go`, plus environment variable injection. |
| **[3. GitHub Actions CI/CD](#3-github-actions-automated-cd-pipeline)** | Automated Deployments | Automated prebuilt artifact deployments on `git push origin main` using `VERCEL_TOKEN`, `VERCEL_ORG_ID`, and `VERCEL_PROJECT_ID`. |
| **[4. Self-Hosting & Multi-Cloud](#4-self-hosting--local-alternatives)** | Infrastructure Portability | 1-Click local stack with Docker Compose (`docker compose up -d`), AWS RDS, Neon, Cloud SQL, and Zero-DB Edge mode. |
| **[5. Troubleshooting & Gotchas](#5-troubleshooting--gotchas)** | Incident Prevention & SOPs | Port `6543` vs `5432` guidelines, special character password URL-encoding, initial admin credentials rotation, and timeouts. |

---

## 📑 Table of Contents
1. [Supabase Setup (Persistent Database)](#1-supabase-setup-persistent-database)
2. [Vercel Setup (Serverless Application)](#2-vercel-setup-serverless-application)
3. [GitHub Actions Automated CD Pipeline](#3-github-actions-automated-cd-pipeline)
4. [Self-Hosting & Local Alternatives](#4-self-hosting--local-alternatives)
5. [Troubleshooting & Gotchas](#5-troubleshooting--gotchas)

---

## 1. Supabase Setup (Persistent Database)

Supabase provides enterprise-grade PostgreSQL with automated connection pooling and physical backups.

### Step 1.1: Create a Supabase Project
1. Log in to [Supabase](https://supabase.com) and click **New Project**.
2. Fill in the project details:
   - **Name**: `flagura-production` (or your preferred name)
   - **Database Password**: Choose a strong, random password and save it securely.
   - **Region**: Select the region closest to your Vercel edge deployment (e.g. `us-east-1`, `eu-west-1`).
   - **Pricing Plan**: Free or Pro tier.
3. Click **Create new project** and wait 1–2 minutes for database provisioning.

---

### Step 1.2: Apply the Database Schema
1. In the Supabase left sidebar, click on **SQL Editor**.
2. Click **New Query**.
3. Copy the complete SQL script from [`supabase/schema.sql`](../../supabase/schema.sql) and paste it into the editor.
4. Click **Run** (or press `Ctrl/Cmd + Enter`).
5. Verify in **Table Editor** that the 5 tables are created:
   - `users` (with unique email index)
   - `sessions` (with user relation and expiration indexes)
   - `feature_flags` (with unique flag key index)
   - `audit_logs` (with timestamp index)
   - `api_keys` (with key index)

---

### Step 1.3: Retrieve the Connection Pooling URI (Port 6543)
Serverless functions spawn hundreds of ephemeral lambdas that can quickly exhaust PostgreSQL connections. Always use Supabase's **Transaction Connection Pooler**:

1. In Supabase sidebar, go to **Project Settings** $\rightarrow$ **Database**.
2. Scroll to the **Connection String** section and click the **Connection Pooling** tab.
3. Select **Mode: Transaction** (Port `6543`).
4. Copy the connection URI:
   ```
   postgres://postgres.[YOUR_PROJECT_REF]:[YOUR_PASSWORD]@aws-0-[REGION].pooler.supabase.com:6543/postgres?sslmode=require
   ```
5. Replace `[YOUR_PASSWORD]` with your real database password.

---

## 2. Vercel Setup (Serverless Application)

Flagura runs natively as a Go Serverless Function on Vercel via [`api/index.go`](../../api/index.go) and [`vercel.json`](../../vercel.json).

### Step 2.1: Import Project to Vercel
1. Log in to [Vercel](https://vercel.com) and click **Add New...** $\rightarrow$ **Project**.
2. Connect your GitHub account and select your `flagura` repository.
3. In **Configure Project**:
   - **Framework Preset**: Select `Other`.
   - **Root Directory**: `./` (default).

---

### Step 2.2: Set Environment Variables in Vercel
In the **Environment Variables** section of the Vercel import screen (or under **Settings $\rightarrow$ Environment Variables**):

| Variable Name | Value | Purpose |
| :--- | :--- | :--- |
| `DATABASE_URL` | `postgres://postgres.[REF]:[PW]@aws-0-[REGION].pooler.supabase.com:6543/postgres?sslmode=require` | Supabase PostgreSQL Connection Pooler |
| `ENVIRONMENT` | `production` | Enforces HSTS and secure cookies |
| `APP_URL` | `https://your-custom-domain.com` (or Vercel URL) | Canonical host for CORS and auth cookies |
| `SECURE_COOKIE`| `true` | Enforces `Secure: true` on all session cookies |

4. Click **Deploy**. Vercel will build and deploy Flagura in ~30 seconds.

---

## 3. GitHub Actions Automated CD Pipeline

The repository includes [`.github/workflows/deploy.yml`](../../.github/workflows/deploy.yml) to automatically test, compile, and deploy prebuilt artifacts to Vercel on every push to `main`.

### Step 3.1: Obtain Vercel Credentials

#### 1. Generate `VERCEL_TOKEN`:
- Go to [Vercel Account Settings $\rightarrow$ Tokens](https://vercel.com/account/tokens).
- Click **Create Token**, name it `flagura-deploy`, and copy the generated token.

#### 2. Obtain `VERCEL_ORG_ID` & `VERCEL_PROJECT_ID`:
Option A: Via Vercel CLI locally:
```bash
npx vercel link
cat .vercel/project.json
```
Option B: From Vercel Dashboard:
- `VERCEL_ORG_ID`: Found in **Vercel Settings $\rightarrow$ General** under *Team ID* or *User ID*.
- `VERCEL_PROJECT_ID`: Found in your **Project Settings $\rightarrow$ General** under *Project ID*.

---

### Step 3.2: Configure GitHub Repository Secrets
1. In your GitHub repository, open **Settings** $\rightarrow$ **Secrets and variables** $\rightarrow$ **Actions**.
2. Click **New repository secret** and add the following 3 secrets:

| Secret Name | Value |
| :--- | :--- |
| `VERCEL_TOKEN` | Token created in Step 3.1 |
| `VERCEL_ORG_ID` | Your Vercel Team / Account ID |
| `VERCEL_PROJECT_ID` | Your Vercel Project ID |

---

### Step 3.3: Enforce Owner & Maintainer Production Approvals
Flagura's deployment workflow is bound to the `production` GitHub Environment. To require manual owner/maintainer sign-off before any code is pushed to production:

1. In your GitHub repository, open **Settings** $\rightarrow$ **Environments**.
2. Click **New environment**, name it `production`, and click **Configure environment**.
3. Under **Deployment protection rules**, check **Required reviewers**.
4. Add the repository owner(s) and lead maintainers as required reviewers.
5. Click **Save protection rules**.

Now, whenever a push to `main` triggers a deployment or release, GitHub Actions will pause and send a review notification to the maintainers, requiring explicit approval before deploying!

---

## 4. Self-Hosting & Local Alternatives

If you or your users prefer to self-host Flagura without Supabase or Vercel:

### Option A: Local Docker Compose (1-Click)
```bash
# Starts local PostgreSQL 16 + Flagura web console with persistent volume
docker compose up -d

# Open console
open http://localhost:3000/dashboard
```

### Option B: Cloud PostgreSQL (AWS RDS, Google Cloud SQL, Neon)
Flagura works with any standard PostgreSQL 14+ database. Simply set `DATABASE_URL`:
```bash
export DATABASE_URL="postgres://user:password@flagura-db.xyz.rds.amazonaws.com:5432/flagura?sslmode=require"
./flagura
```

### Option C: Zero-Database In-Memory Mode
If `DATABASE_URL` is omitted, Flagura boots instantly using its in-memory deterministic engine with zero cloud dependencies.

---

## 5. Troubleshooting & Gotchas

### 1. `[WARN] Failed to connect to Supabase PostgreSQL...`
- **Cause**: Using direct connection port `5432` on serverless instead of pooler port `6543`, or password has special characters that are not URL-encoded.
- **Fix**: Ensure the connection string uses port **`6543`** with `sslmode=require`. If your password contains characters like `#`, `@`, or `/`, URL-encode them (e.g. `@` $\rightarrow$ `%40`).

### 2. Initial Admin Login
- Default seed account:
  - **Email**: `dhawal@flagura.dev`
  - **Password**: `password123`
- Immediately upon first login, navigate to your database or account settings to update the password hash.

### 3. Vercel Function Timeout
- Go serverless functions resolve flag evaluations in `< 1ms`. If requests time out, verify that your Supabase database is not paused due to inactivity in free tier.
