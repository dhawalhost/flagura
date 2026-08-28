# 🌐 Multi-Environment Workflows

Flagura provides full multi-environment isolation across **Production**, **Staging**, and **Development**. Each environment has its own independent master switch, rollout strategy, targeting rules, and audit logs.

---

## 🏢 Environment Topology

```
┌─────────────────────────────────────────────────────────────┐
│                       FLAGURA PLATFORM                      │
├─────────────────┬─────────────────────┬─────────────────────┤
│   DEVELOPMENT   │       STAGING       │     PRODUCTION      │
├─────────────────┼─────────────────────┼─────────────────────┤
│ • Local Sandbox │ • QA & CI Testing   │ • Live End-Users    │
│ • 100% Rollouts │ • Employee Previews │ • Canary Rollouts   │
│ • Rapid testing │ • Integration Verif │ • P99 Monitoring    │
└─────────────────┴─────────────────────┴─────────────────────┘
```

---

## 🚦 Recommended Promotion Workflow

```
[Write Code & Flag]
        │
        ▼
[Development Environment]
  • Master Switch: ON
  • Strategy: 100% Boolean (Local testing)
        │
        ▼
[Staging Environment]
  • Master Switch: ON
  • Targeting Rule: Whitelist `@company.com` (Dogfooding)
  • Strategy: 0% General Rollout
        │
        ▼
[Production Environment - Canary]
  • Master Switch: ON
  • Targeting Rule: Whitelist Internal VIP Accounts
  • Strategy: 5% - 10% Canary Rollout
        │
        ▼
[Production Environment - Full Release]
  • Strategy: 100% Rollout
```

---

## 🛡️ Production Safety Safeguards

1. **Environment Protection Gates**: Changing production flags or modifying database schemas requires authorized role permissions (`admin`).
2. **Deterministic Context Scoping**: When client requests evaluate flags, specifying `environment: "production"` or `environment: "staging"` ensures complete separation of configuration without cross-environment leaks.
3. **Instant Circuit Breaking**: If an incident occurs in Production, disabling the flag in Production has zero effect on active Staging or Development workflows.
