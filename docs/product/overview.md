# 🌟 Product Overview & Core Concepts

Flagura is a modern, high-performance feature flagging and progressive delivery engine engineered for **sub-microsecond deterministic evaluation**.

---

## 🎯 The Problem Flagura Solves

Traditional SaaS feature flag providers require client applications to make remote HTTP requests (~20ms to 100ms) on every feature check, or suffer from complex synchronization protocols that introduce unpredictable latency and degrade Core Web Vitals (CWV).

Flagura eliminates this overhead:
1. **Deterministic Hashing**: Rollout percentages are computed using mathematical **64-bit FNV-1a sticky hashing**. Users land in the exact same rollout bucket every single time without requiring database lookups.
2. **Sub-Microsecond Latency**: Local in-memory evaluations execute in **~85 nanoseconds across all storage backends** (SQLite, PostgreSQL, and In-Memory Edge Store), enabling millions of checks per second without blocking the application thread or paying database I/O penalties.
3. **Zero External Dependencies**: Runs as a single binary with embedded **SQLite (WAL mode)**, distributed **PostgreSQL**, or a zero-dependency **In-Memory Edge Store**.
4. **Decoupled Architecture**: Flag configurations are managed centrally in the Flagura control plane and evaluated locally in-process anywhere (Microservices, Edge Workers, Lambdas, or Frontend apps).

---

## 🧠 Mental Model & Key Primitives

Flagura organizes feature delivery around 5 core primitives:

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Feature Flag │ ──► │ Environments │ ──► │ Strategies   │
└──────────────┘     └──────────────┘     └──────────────┘
                                                 │
                                                 ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Evaluation  │ ◄── │  Context     │ ◄── │ Targeting    │
│  Result      │     │  Attributes  │     │ Rules        │
└──────────────┘     └──────────────┘     └──────────────┘
```

1. **Feature Flag**: A named configuration toggle (e.g. `ai-smart-search`, `new-checkout-flow`) that dictates how a feature behaves in code.
2. **Environments**: Isolated operational realms (`Production`, `Staging`, `Development`). A flag can be 100% active in Staging while remaining at 10% in Production.
3. **Rollout Strategies**: The mathematical policy for serving variants (Boolean toggle, Percentage rollout, Multivariate experiment).
4. **Targeting Rules**: Predicate conditions that match user attributes (e.g. `email endsWith @company.com`, `tier == enterprise`).
5. **Context Attributes**: Key-value data passed by client applications at evaluation time (User ID, Email, Role, Country, etc.).

---

## ⚡ Execution Lifecycle

When an evaluation request is triggered:

```
[Request Arrives with Context]
             │
             ▼
1. Is Flag Enabled for this Environment?
      ├─► NO  ──► Return (Enabled: false, Reason: MASTER_KILL_SWITCH_DISABLED)
      └─► YES
             │
             ▼
2. Do any Attribute Targeting Rules Match?
      ├─► YES ──► Return Matched Variant (Reason: TARGETING_RULE_MATCH)
      └─► NO
             │
             ▼
3. Evaluate Strategy
      ├─► Boolean Strategy       ──► Return Flag State (Reason: DEFAULT_RULE)
      ├─► Percentage Strategy    ──► Compute FNV-1a Hash Bucket (0-99)
      │                                ├─► Hash < Percentage  ──► Enabled: true (PERCENTAGE_ROLLOUT)
      │                                └─► Hash >= Percentage ──► Enabled: false (CONTROL_GROUP)
      └─► Multivariate Strategy  ──► Map Hash to Weighted Variant Buckets
```

---

## 🛡️ Master Kill-Switch Circuit Breaker

Every flag in Flagura includes an **instant 1-click master kill-switch**. If a newly rolled out feature triggers production errors or memory leaks:
- Flip the toggle switch to `OFF` in the console.
- Within milliseconds, all clients immediately drop to the safe control group with **zero code deployments or restarts required**.

---

## 🧹 Automated Code Hygiene & Technical Debt Elimination

Unlike legacy flag platforms where obsolete flags rot inside production codebases, Flagura actively manages flag lifecycles:
- **Automatic Stale Detection**: Flags that are 100% rolled out in production with no custom rules are flagged as `READY_FOR_CLEANUP`.
- **In-App Refactoring Guidance**: Interactive cleanup assistant shows exact before/after code diffs in **Go**, **TypeScript**, and **Python**.
- **CLI Static Scanning**: Run `flagura scan .` in your repositories to locate all flag occurrences in code and integrate with CI/CD gates (`--fail-on-stale`).

👉 **[Read Code Hygiene & Flag Debt Guide](code-hygiene-and-flag-debt.md)**

---

## 🌐 CNCF OpenFeature Standard Native

Flagura provides drop-in OpenFeature Providers for **Go**, **TypeScript**, and **Python**:
- **Zero Vendor Lock-In**: Code against standard OpenFeature APIs.
- **Real-Time Config Bus**: Automatically emits `ProviderReady` and `ProviderConfigChange` events via SSE streams.
- **Polyglot Examples**: Complete working applications available in [`examples/`](../../examples/README.md).

👉 **[Read OpenFeature Integration Guide](../integrations/openfeature.md)**

