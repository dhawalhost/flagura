# 📈 Analytics, Performance Benchmarking & Audit Logs

Flagura provides real-time visibility into flag performance, rollout distribution gauges, evaluation latency, and an immutable audit trail of all configuration changes.

---

## 📊 1. Bento Grid Analytics & Rollout Gauges

The developer console features an interactive Bento Grid dashboard showing:
- **Total Flags Active**: Breakdown across Boolean, Percentage, and Multivariate strategies.
- **Rollout Gauges**: Real-time visualization of current canary percentages (0% to 100%).
- **Evaluation Latency Meter**: Sub-microsecond P99 evaluation latencies measured in nanoseconds.
- **Environment Distribution**: Active vs disabled flags across Production, Staging, and Development.

---

## ⚡ 2. Real-Time In-Process Latency Benchmarks

Flagura includes a built-in benchmark runner (`POST /api/v1/benchmark`):
- Executes **10,000 to 100,000 deterministic in-memory evaluations** in real-time.
- Measures P50, P90, and P99 latency percentiles.
- Demonstrates that evaluation hot paths take **`< 1 microsecond (300ns – 800ns)`**, proving zero impact on web application request cycles.

---

## 📝 3. Immutable Audit Trails

Every mutating action in Flagura is logged with:
- **Actor Name & Email**: Who initiated the change (e.g. `dhawal@flagura.dev`).
- **Action Type**: `FLAG_CREATED`, `FLAG_TOGGLED`, `ROLLOUT_UPDATED`, `FLAG_DELETED`.
- **Target Entity**: The exact flag key and environment modified.
- **Timestamp & Details**: ISO8601 timestamp and state delta.

### Accessing Audit Logs
- **Via Console**: Click the **Audit Logs** button in the dashboard to open the audit trail modal.
- **Via REST API**:
  ```bash
  curl -s https://flagura.dev/api/v1/audit-logs \
    -H "Authorization: Bearer <TOKEN>"
  ```
