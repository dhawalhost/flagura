# 🧹 Automated Code Hygiene & Flag Debt Management

Feature flags are incredible for shipping safely, but left unmanaged, they become **technical debt**:
- Stale flags cluttering production codebases.
- Complex nested conditional checks (`if flagA && flagB`) that engineers fear deleting.
- Dead code paths for abandoned experiments remaining in production indefinitely.

Flagura is built with a **first-class Code Hygiene Engine** that proactively identifies stale flags, shows exact refactoring cleanup diffs, and scans your local Git repositories.

---

## 🚦 Flag Lifecycle Health States

Flagura continuously analyzes every flag's environment settings and targeting rule configurations:

| Health Status | Badge | Condition | Recommended Action |
| :--- | :--- | :--- | :--- |
| **`ACTIVE`** | 🟢 Active | Flag is running canary rollout percentage or active custom targeting rules. | Monitor canary health; increment percentage as confidence increases. |
| **`READY_FOR_CLEANUP`** | 🧹 Ready for Cleanup | Flag is **100% rolled out** in production with 0 custom rules. | Feature is permanent! Remove the conditional check from codebase. |
| **`DEAD_FLAG`** | ⚠️ Dead Flag | Flag is disabled or kill-switched (0% traffic) in production. | Experiment abandoned or permanently cancelled. Safely purge dead code. |

---

## 🖥️ Interactive Web UI Cleanup Assistant

In the **Flag Matrix** (`/dashboard`):
1. Use the **Health Filter** dropdown to isolate all flags that are `Stale (100% Launched)` or `Dead (0% / Disabled)`.
2. Click any **"Ready for Cleanup"** badge to open the **Automated Code Hygiene & Cleanup Modal**:
   - **Diagnostic**: Detailed explanation of why the flag is classified as technical debt.
   - **Refactoring Diff**: A color-coded before/after cleanup diff in **Go**, **TypeScript**, or **Python**.
   - **One-Click Copy**: Copy the simplified permanent code snippet into your clipboard.
   - **Delete Flag**: Safely archive/delete the flag once your PR merges to remove it from the console.

---

## 💻 CLI Codebase Scanning (`flagura scan`)

The Flagura Developer CLI includes an automated static analysis scanner that walks your Git repository and identifies all flag occurrences in code:

```bash
# Scan current directory for flag keys in code
flagura scan .
```

Example Output:
```text
🔍 Flagura Code Hygiene Scanner
   Directory: .
   Scanned: 142 files (.go, .ts, .py, .rs)

--------------------------------------------------------------------------------
FLAG KEY           HEALTH STATE       OCCURRENCES   ACTION
--------------------------------------------------------------------------------
ai-smart-search    ✅ Active           4 in 2 files  Keep active (canary rollout)
checkout-v2        🧹 Ready for Cleanup 8 in 3 files  100% launched: safe to remove
legacy-tax-calc    ⚠️ Dead Flag        2 in 1 files  Kill-switched: purge dead code
--------------------------------------------------------------------------------

Summary:
   Total Flags Found: 3
   Flags Ready for Cleanup: 1
   Dead Flags: 1

💡 Run 'flagura cleanup <key>' to review refactoring diffs.
```

### CI/CD Quality Gate
You can add Flagura's hygiene scanner to your CI/CD pipeline to prevent flag debt from accumulating:
```yaml
# GitHub Actions / GitLab CI
- name: Check Feature Flag Hygiene
  run: |
    flagura scan ./src --fail-on-stale
```
