# 🚩 Feature Flags Deep Dive

Feature flags in Flagura allow engineering teams to separate code deployment from feature release. This enables trunk-based development, dark launching, gradual percentage rollouts, and multi-variant experimentation.

---

## 🏷️ Flag Types & Strategies

Flagura supports 3 foundational flag strategies:

### 1. 🔘 Boolean Flags (Simple Toggles)
Binary switches that are either `true` (Enabled / Treatment) or `false` (Disabled / Control).

- **Best for**: Feature releases, maintenance banners, circuit breakers, experimental code paths.
- **Evaluation**: Resolves instantly based on environment state and targeting overrides.

```json
{
  "key": "v2-search-pipeline",
  "type": "boolean",
  "environments": {
    "production": {
      "enabled": true,
      "strategy": "boolean"
    }
  }
}
```

---

### 2. 📊 Percentage Rollouts (Canary Releases)
Gradual exposure of a feature to a specific proportion of your user base (e.g. 5%, 25%, 50%, 100%).

- **Best for**: Canary deployments, load testing backend capacity, mitigating blast radius of breaking changes.
- **Deterministic Stickiness**: Uses **64-bit FNV-1a hashing** combined with the `user_id` and `flag_key`. A user assigned to the 10% bucket will consistently stay in that bucket as the rollout increases to 25% and 50%.

```json
{
  "key": "ai-smart-search",
  "type": "boolean",
  "environments": {
    "production": {
      "enabled": true,
      "strategy": "percentage",
      "percentage": 25
    }
  }
}
```

---

### 3. 🎨 Multivariate Flags (A/B/n Experiments)
Flags that serve multiple variations (strings, numbers, JSON objects) with configurable weights.

- **Best for**: UI redesigns, pricing tier experiments, algorithm parameter tuning.
- **Bucket Allocation**: Sum of weights must equal 100%. Users are deterministically mapped to a variant based on their hash value.

```json
{
  "key": "beta-dark-theme",
  "type": "multivariate",
  "environments": {
    "production": {
      "enabled": true,
      "strategy": "multivariate",
      "variants": [
        { "key": "dark-blue", "value": "dark-blue", "weight": 50 },
        { "key": "dark-slate", "value": "dark-slate", "weight": 50 }
      ]
    }
  }
}
```

---

## 🔄 Flag Lifecycle Best Practices

1. **Create in Development**: Test feature flags with custom contexts in the local or development environment.
2. **Promote to Staging**: Enable for internal team members using targeting rules (e.g. `@company.com` email filter).
3. **Canary Rollout in Production**: Start at 5% or 10% in production. Monitor error rates, server latency, and user feedback.
4. **Scale to 100%**: Increment percentage rollout (25% $\rightarrow$ 50% $\rightarrow$ 100%).
5. **Retire & Clean Up**: Once a feature is permanently adopted, remove the flag condition from your codebase and delete the flag in Flagura to eliminate technical debt.
