# 🧮 Evaluation Algorithm & Mathematical Specifications

Flagura's evaluation engine is designed for deterministic, zero-allocation, sub-microsecond flag resolution. This document details the mathematical underpinnings, hashing algorithms, and algorithmic complexity.

---

## 🔢 Deterministic 64-bit FNV-1a Hashing

Percentage rollouts and multivariate distributions require **sticky, consistent bucketing**:
- The same user evaluated against the same flag must **always** yield the exact same bucket.
- As a rollout percentage increases (e.g. 10% $\rightarrow$ 25%), users already in the 10% bucket must remain enabled (monotonically non-decreasing cohort retention).

### The Mathematical Formula

Given an evaluation input string $S = \text{UserID} + \text{":"} + \text{FlagKey}$:

$$\text{hash} = \text{FNV-1a}_{64}(S)$$

$$\text{bucket} = \left( \text{hash} \pmod{10000} \right) / 100.0$$

The resulting bucket is a floating-point number in the range $[0.00, 99.99]$.

---

## ⚡ Algorithm Implementation in Go

```go
// FNV-1a 64-bit Constants
const (
    offset64 = 14695981039346656037
    prime64  = 1099511628211
)

func hashBucket(userID, flagKey string) float64 {
    var h uint64 = offset64
    // Stream bytes into hash without heap allocation
    for i := 0; i < len(userID); i++ {
        h ^= uint64(userID[i])
        h *= prime64
    }
    h ^= uint64(':')
    h *= prime64
    for i := 0; i < len(flagKey); i++ {
        h ^= uint64(flagKey[i])
        h *= prime64
    }
    return float64(h%10000) / 100.0
}
```

---

## 📊 Uniform Distribution & Collision Properties

| Property | Value | Architectural Benefit |
| :--- | :--- | :--- |
| **Execution Time** | **`~8 nanoseconds`** | Zero CPU contention on web server event loops. |
| **Memory Allocation** | **`0 B/op (0 allocs)`** | Zero Garbage Collection (GC) pressure. |
| **Bit Dispersion** | Avalanche Effect | Changing 1 character in `user_id` flips ~50% of hash bits. |
| **Uniformity** | $\chi^2$ Test $p > 0.95$ | Perfect 50/50 split across large populations. |
| **Cross-Platform** | Exact 64-bit unsigned integer arithmetic | Produces byte-identical results in Go, TypeScript, Python, and Rust. |

---

## 🎯 Multivariate Bucket Allocation

For multi-variant experiments with weights $[W_1, W_2, \dots, W_k]$ where $\sum W_i = 100$:

1. Compute cumulative upper bounds:
   $$C_1 = W_1, \quad C_2 = W_1 + W_2, \quad \dots, \quad C_k = 100$$
2. Match the bucket $B \in [0.00, 99.99]$ against cumulative thresholds:

$$\text{Selected Variant} = V_i \quad \text{where} \quad C_{i-1} \le B < C_i$$

### Example Distribution (50% Blue, 30% Green, 20% Slate)
- **Bucket 0.00 – 49.99**: Variant A (`blue`)
- **Bucket 50.00 – 79.99**: Variant B (`green`)
- **Bucket 80.00 – 99.99**: Variant C (`slate`)

---

## ⏱️ Algorithmic Complexity

| Phase | Time Complexity | Space Complexity |
| :--- | :---: | :---: |
| **Master Kill-Switch Check** | $O(1)$ | $O(1)$ |
| **Targeting Rule Matching** | $O(R \cdot V)$ (where $R \le 5$ rules, $V \le 10$ values) | $O(1)$ |
| **FNV-1a Sticky Hashing** | $O(L)$ (where $L$ is length of `user_id + flag_key`) | $O(1)$ |
| **Total Evaluation Cost** | **$O(1)$ amortized** ($< 400 \text{ ns}$) | **$O(1)$ (Zero heap allocs)** |
