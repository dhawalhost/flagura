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

## ⚙️ Supported Operators

Flagura provides a rich set of string, set, regex, and numerical operators across the server engine and client SDKs:

| Category | Operator | Aliases | Description | Example |
| :--- | :--- | :--- | :--- | :--- |
| **Equality** | `equals` | `eq`, `==` | Exact string match | `role equals admin` |
| | `not_equals` | `neq`, `!=` | Inequality string match | `tier not_equals free` |
| **Substrings** | `contains` | | Substring presence | `email contains @corp` |
| | `not_contains` | | Substring absence | `email not_contains @competitor` |
| | `starts_with` | | Prefix match | `user_id starts_with beta_` |
| | `ends_with` | | Suffix match | `email ends_with @company.com` |
| **Sets** | `in` | | Set inclusion (comma-separated or list) | `country in US,CA,GB` |
| | `not_in` | | Set exclusion | `country not_in CN,RU` |
| **Regular Expressions** | `matches_regex` | `regex` | Regular expression match (case-sensitive by default) | `email matches_regex ^[a-z]+@corp\.com$` |
| **Numeric Comparisons** | `greater_than` | `gt`, `>` | Strict numeric greater than | `app_version greater_than 2.4` |
| | `greater_than_or_equal` | `gte`, `>=` | Numeric greater than or equal | `score >= 100` |
| | `less_than` | `lt`, `<` | Strict numeric less than | `latency < 250` |
| | `less_than_or_equal` | `lte`, `<=` | Numeric less than or equal | `age <= 65` |

> [!NOTE]
> **Regex Case Sensitivity**: Regular expressions are case-sensitive by default to respect exact author patterns. For case-insensitive regex matching, include the standard `(?i)` flag at the beginning of your pattern (e.g. `(?i)^prod-`).

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
