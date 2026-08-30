# ⚡ Flagura Python SDK

Official Python client for the **Flagura Feature Flag Platform**, supporting:
- High-performance evaluations
- **Real-Time SSE Flag Streaming (`<5ms` sync)**
- **CNCF OpenFeature Provider**
- FastAPI, Django, Flask, Celery, and AI Agent compatibility

---

## 📦 Installation

```bash
pip install flagura
```

---

## 🚀 Quickstart

### 1. Direct Client with Real-Time SSE Streaming

```python
from flagura import FlaguraClient, EvaluationContext

# Initialize client with real-time SSE streaming
client = FlaguraClient(
    endpoint="https://flagura.dhawalhost.com",
    api_key="your-api-key",
    default_environment="production",
    enable_streaming=True, # <5ms live flag updates
)

# Register real-time change listener
client.on_update(lambda flags: print(f"Flags updated in real-time! Count: {len(flags)}"))

# Evaluate flag
context = EvaluationContext(
    user_id="usr_dhawal_01",
    email="dhawal@flagura.dev",
    tier="enterprise",
)

if client.is_enabled("ai-smart-search", context):
    variant = client.get_variant("ai-smart-search", context)
    print(f"AI Smart Search is ON! Variant: {variant}")

# Cleanup on shutdown
client.close()
```

---

### 2. OpenFeature Universal Provider

```python
from openfeature import api
from openfeature.evaluation_context import EvaluationContext
from flagura.openfeature_provider import FlaguraOpenFeatureProvider

# Register Flagura as OpenFeature provider
api.set_provider(FlaguraOpenFeatureProvider(
    endpoint="https://flagura.dhawalhost.com",
    api_key="your-api-key",
    enable_streaming=True,
))
of_client = api.get_client()

# Evaluate with OpenFeature standard APIs
ctx = EvaluationContext(targeting_key="usr_dhawal_01", attributes={"email": "dhawal@flagura.dev"})
is_enabled = of_client.get_boolean_value("ai-smart-search", False, ctx)
```

---

## 📄 License
MIT
