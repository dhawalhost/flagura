# 🎯 Attribute-Based Targeting Rules & Segmentation

Targeting rules allow you to deliver specific flag variants or bypass percentage rollouts for select users, internal teams, beta testers, or enterprise customers.

---

## 🔍 How Targeting Rules Work

Targeting rules are evaluated in **priority order (top to bottom)** before general percentage rollouts:
- If a rule matches the incoming context, the evaluation immediately short-circuits and serves the specified variant.
- If no rules match, the evaluation falls through to the environment's default strategy (e.g. percentage rollout).

---

## 📋 Supported Attributes

| Attribute Key | Type | Description | Common Use Cases |
| :--- | :--- | :--- | :--- |
| **`user_id`** | String | Unique identifier for the user / account | Whitelisting VIP users, beta testers, canary QA accounts |
| **`email`** | String | User's email address | Whitelisting internal employees by domain (`@flagura.dev`) |
| **`country`** | String | Two-letter ISO country code (`US`, `GB`, `IN`, `DE`) | Geo-gated features, regional compliance (e.g. GDPR) |
| **`role`** | String | User role (`admin`, `editor`, `viewer`, `beta_tester`) | Restricting admin preview features to staff |
| **`tier`** | String | Account subscription level (`free`, `pro`, `enterprise`) | Tier-gated premium features |
| **`custom`** | Map | Dynamic key-value pairs (`app_version`, `device`, etc.) | Mobile app version gating, custom tenant routing |

---

## 💡 Real-World Examples

### 1. Internal Team Whitelist
Give all employees access to experimental AI features ahead of public launch:

```json
{
  "id": "rule_internal_staff",
  "name": "Staff & Internal Domain Whitelist",
  "attribute": "email",
  "operator": "ends_with",
  "values": ["@flagura.dev", "@company.com"],
  "serve_variant": "treatment",
  "enabled": true
}
```

---

### 2. Enterprise Tier Feature Gating
Enable advanced analytics exclusively for Enterprise customers:

```json
{
  "id": "rule_enterprise_tier",
  "name": "Enterprise Subscription Gating",
  "attribute": "tier",
  "operator": "in",
  "values": ["enterprise", "enterprise_plus"],
  "serve_variant": "treatment",
  "enabled": true
}
```

---

### 3. Geographic Beta Launch
Roll out modern checkout flow specifically to users in the United States and Canada:

```json
{
  "id": "rule_na_region",
  "name": "North America Regional Launch",
  "attribute": "country",
  "operator": "in",
  "values": ["US", "CA"],
  "serve_variant": "treatment",
  "enabled": true
}
```

---

## ⚡ Context Passing in Code

When calling the Flagura API or SDK, simply include the context attributes in your evaluation payload:

```go
result, err := client.Evaluate(ctx, "ai-smart-search", client.Context{
    UserID:  "usr_dhawal_01",
    Email:   "dhawal@flagura.dev",
    Country: "US",
    Role:    "admin",
    Tier:    "enterprise",
})
```
