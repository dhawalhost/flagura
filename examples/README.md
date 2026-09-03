# 🚀 Flagura SDK Integration & OpenFeature Examples

This directory contains clean, runnable, copy-paste ready integration examples demonstrating both the **Native Flagura SDKs** and the **CNCF OpenFeature Provider** across multiple programming languages.

---

## 📂 Available Examples

| Directory | Language / Runtime | Features Demonstrated |
| :--- | :--- | :--- |
| [`go/`](./go) | **Go 1.22+** | Native in-process evaluator (`<85ns`), SSE streaming synchronization, `openfeature.FeatureProvider`. |
| [`typescript/`](./typescript) | **TypeScript / Node.js** | Native typed client, `@openfeature/server-sdk` provider, context mapping. |
| [`python/`](./python) | **Python 3.10+** | Native `FlaguraClient`, `openfeature` standard API, variant resolution. |
| [`rust/`](./rust) | **Rust 1.70+** | Async Tokio client, zero-allocation context building, microsecond evaluation. |

---

## 🏃‍♂️ Quickstart

### Prerequisites
Make sure your Flagura server is running locally:
```bash
# In the repository root
make dev
# Server listening at http://localhost:3000
```

---

### 1. Go Example (Native + OpenFeature)
```bash
cd examples/go
go run main.go
```
Expected output:
```text
🚀 Flagura Go Integration Example (Native + OpenFeature)
--- 1. Native Flagura Client (In-Process Fast Path) ---
Flag: ai-smart-search | Enabled: true | Variant: claude-3-opus | Latency: 78 ns | Reason: STRATEGY_PERCENTAGE

--- 2. CNCF OpenFeature Go Provider ---
OpenFeature BooleanValue('ai-smart-search'): true
OpenFeature StringValueDetails: Value=claude-3-opus | Variant=claude-3-opus | Reason=SPLIT
```

---

### 2. TypeScript Example (Native + OpenFeature)
```bash
cd examples/typescript
npm install
npm start
```
Expected output:
```text
🚀 Flagura TypeScript Integration Example (Native + OpenFeature)
--- 1. Native Flagura Client ---
Flag: ai-smart-search
Enabled: true
Variant: claude-3-opus
Reason: STRATEGY_PERCENTAGE

--- 2. CNCF OpenFeature Provider ---
OpenFeature getBooleanValue('ai-smart-search'): true
OpenFeature getStringDetails: Value="claude-3-opus", Variant="claude-3-opus", Reason="SPLIT"
```

---

### 3. Python Example (Native + OpenFeature)
```bash
cd examples/python
python3 example.py
```
Expected output:
```text
🚀 Flagura Python Integration Example (Native + OpenFeature)
--- 1. Native Flagura Client ---
Flag: ai-smart-search
Enabled: True
Variant: claude-3-opus
Reason: STRATEGY_PERCENTAGE

--- 2. CNCF OpenFeature Provider ---
OpenFeature get_boolean_value('ai-smart-search'): True
OpenFeature get_string_details: Value=claude-3-opus, Variant=claude-3-opus, Reason=SPLIT
```

---

### 4. Rust Example (Native Async)
```bash
cd examples/rust
cargo run
```
Expected output:
```text
🚀 Flagura Rust Integration Example
✨ Feature flag 'ai-smart-search' is ENABLED for usr_alex_42
Evaluation Details: Flag=ai-smart-search, Enabled=true, Variant=claude-3-opus, Reason=STRATEGY_PERCENTAGE, Latency=41us
```

---

## 🛡️ CNCF OpenFeature Standard Benefits
Using the Flagura OpenFeature Provider allows your engineering teams to:
1. **Prevent Vendor Lock-In**: Write standard OpenFeature evaluation code across your applications. If your infrastructure evolves, switch providers with 1 line of configuration.
2. **Standardized Context**: Universal evaluation attributes (`targetingKey`, `userId`, `email`, `role`, `tier`) are consistently passed to targeting rules.
3. **Real-time SSE Config Change Bus**: OpenFeature events (`ProviderReady`, `ProviderConfigChange`) fire automatically whenever feature flags are updated in the Flagura console or via GitOps.
