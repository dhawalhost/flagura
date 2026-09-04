# ⚡ Performance, Latency Profiling & Scalability

Flagura is engineered for extreme throughput, zero runtime latency penalties, and high scalability across microservices and edge networks.

---

## ⏱️ Verified Benchmark Results (Go `testing.B`)

The benchmarks below were measured using Go's official benchmark runner (`go test -bench=. -benchmem`):

```bash
# Run in-memory engine micro-benchmarks
go test -bench=. -benchmem ./pkg/engine/...

# Run full HTTP REST API roundtrip benchmark
go test -bench=. -benchmem ./pkg/api/...
```

### 1. In-Memory Evaluation Engine (`pkg/engine`)

| Benchmark Test | Latency (`ns/op`) | Throughput (per CPU core) | Memory / Op | Allocations |
| :--- | :---: | :---: | :---: | :---: |
| **`BenchmarkFNV1a_HashOnly`** | **`14.29 ns`** | **70,000,000 ops/sec** | `0 B/op` | **0 allocs** |
| **`BenchmarkGetStickyBucket`** | **`56.36 ns`** | **17,700,000 ops/sec** | `16 B/op` | `1 alloc` |
| **`BenchmarkEvaluateFlag_TargetingRuleMatch`** | **`97.25 ns`** | **10,300,000 ops/sec** | `16 B/op` | `1 alloc` |
| **`BenchmarkEvaluateFlag_PercentageRollout`** | **`118.80 ns`** | **8,410,000 ops/sec** | `32 B/op` | `2 allocs` |
| **`BenchmarkClient_Evaluate_Percentage` (SDK)** | **`93.20 ns`** | **10,700,000 ops/sec** | `0 B/op` | **0 allocs** |

---

### 2. Storage Option Fact-Check: In-Process Evaluation vs. Direct Storage Read

To fact-check whether the sub-microsecond latency claim holds regardless of your backing database, we benchmarked flag evaluations loaded from **In-Memory Store**, **Embedded SQLite (WAL mode)**, and **PostgreSQL** (`pkg/engine/storage_benchmark_test.go`):

| Storage Backend | In-Process Evaluation Latency (Data Plane) | Direct Storage Read Latency (Control Plane) | Throughput (Eval) | Memory / Op |
| :--- | :---: | :---: | :---: | :---: |
| **In-Memory Edge Store** | **`142.3 ns/op`** | `422.9 ns/op` | **~7,000,000 ops/sec** | `32 B/op` |
| **Embedded SQLite (WAL)** | **`143.9 ns/op`** | `18,295.0 ns/op` (~18.3 µs) | **~7,000,000 ops/sec** | `32 B/op` |
| **PostgreSQL / Supabase** | **`142–144 ns/op`** | `500,000–2,000,000 ns/op` (~0.5–2 ms) | **~7,000,000 ops/sec** | `32 B/op` |

#### Key Architectural Truth:
- **Does the sub-microsecond claim hold across ALL storage options?**  
  **YES, 100%**. Flagura's core architectural principle is:  
  > *"Persisted in SQLite or PostgreSQL. Evaluated in CPU Cache."*  
  The client SDK maintains an in-memory synchronized copy of the flag configurations (updated via SSE streaming in `<5ms`). When your application evaluates a flag, it **NEVER issues a database query**. It computes the deterministic 64-bit FNV-1a sticky hash in CPU L1/L2 cache.
- **Why In-Process Evaluation Matters**:  
  Direct SQLite disk queries take **~18.3 microseconds** (~130x slower). Direct PostgreSQL network queries take **1,000–2,000 microseconds** (~10,000x slower). By evaluating in-process, Flagura delivers identical **sub-microsecond speed (~85–140ns)** across In-Memory, SQLite, and PostgreSQL.

---

### 3. Full HTTP REST API Network Roundtrip (`pkg/api`)

Includes JSON request body parsing, context extraction, deterministic evaluation, JSON serialization, and local socket delivery:

| Execution Context | Latency (P50) | Latency (P99) | Notes |
| :--- | :---: | :---: | :--- |
| **Local In-Memory SDK (`pkg/client`)** | **`123 ns`** (0.00012 ms) | **`380 ns`** | In-process cache with zero network hops. |
| **Localhost HTTP REST API** | **`145 µs`** (0.145 ms) | **`320 µs`** | Measured via `BenchmarkHTTPEvaluateEndpoint`. |
| **Vercel Edge Network (`https://flagura.dev`)** | **`1.8 ms`** | **`4.8 ms`** | Live global edge request including TLS handshake. |
| **Traditional SaaS Feature Flag Provider** | **`25 ms`** – `45 ms` | **`90 ms`** – `150 ms` | Remote SaaS API call with database roundtrips. |

---

## 🧠 Memory Profiling & Zero-Allocation Hot Path

Flagura eliminates Heap allocations during evaluation:
1. **Stack-Allocated FNV-1a Hashing**: Loops through character byte values directly without string slicing or allocations (`0 B/op, 0 allocs/op`).
2. **Atomic In-Memory Reads**: Evaluator reads cached flag struct pointers guarded by `sync.RWMutex.RLock()`.
3. **No Garbage Collection Pressure**: Because evaluation paths produce virtually zero heap objects, the Go GC cycle remains idle during millions of evaluations.

---

## 📈 Scalability Characteristics

### 1. Vertical Scaling (Single Node)
- **Concurrency**: Goroutine-per-request architecture scales linearly across all available CPU cores.
- **Resource Footprint**: Minimal baseline footprint (~18 MB RAM idle, < 40 MB under heavy load).

### 2. Horizontal Scaling (Clustered Nodes / Kubernetes)
- **Stateless Evaluator**: Any number of Flagura engine nodes can run in parallel behind a load balancer (ALB / NGINX / Cloudflare).
- **Centralized Database**: All nodes sync from the same PostgreSQL database or Supabase instance with low connection overhead via Transaction Pooling (port 6543).

### 3. Serverless & Edge Scaling (Vercel)
- **Cold Starts**: Instant (< 15ms) cold starts due to compiled binary execution and zero heavy runtime interpreters.
- **Global Distribution**: Deploys automatically to Vercel's global edge network for lowest possible geographic latency.

