"""
Flagura Python Integration Example (Native + OpenFeature)

Demonstrates:
1. Native FlaguraClient evaluation with typed context and streaming sync.
2. Vendor-agnostic CNCF OpenFeature Provider integration.
"""

import os
import sys
from pathlib import Path

# Add python sdk directory to sys.path for direct execution
sdk_path = str(Path(__file__).resolve().parent.parent.parent / "sdks" / "python")
if sdk_path not in sys.path:
    sys.path.insert(0, sdk_path)

from flagura import FlaguraClient, EvaluationContext
from flagura.openfeature_provider import FlaguraOpenFeatureProvider

def main():
    endpoint = os.getenv("FLAGURA_ENDPOINT", "http://localhost:3000")
    api_key = os.getenv("FLAGURA_API_KEY", "flg_live_demo_key_example")

    print("🚀 Flagura Python Integration Example (Native + OpenFeature)")
    print(f"   Endpoint: {endpoint}\n")

    # =========================================================================
    # 1. Native High-Performance Flagura Client
    # =========================================================================
    print("--- 1. Native Flagura Client ---")
    client = FlaguraClient(
        endpoint=endpoint,
        api_key=api_key,
        default_environment="production",
        enable_streaming=False,
    )

    user_ctx = EvaluationContext(
        user_id="usr_david_07",
        email="david@company.com",
        country="US",
        role="engineer",
        tier="pro",
        custom={"beta_tester": True}
    )

    is_enabled = client.is_enabled("ai-smart-search", user_ctx)
    variant = client.get_variant("ai-smart-search", user_ctx)
    details = client.evaluate("ai-smart-search", user_ctx)

    print(f"Flag: ai-smart-search")
    print(f"Enabled: {is_enabled}")
    print(f"Variant: {variant}")
    print(f"Reason: {details.reason}")
    print(f"Bucket: {details.bucket}%\n")

    # =========================================================================
    # 2. CNCF OpenFeature Provider
    # =========================================================================
    print("--- 2. CNCF OpenFeature Provider ---")
    try:
        from openfeature import api
        from openfeature.evaluation_context import EvaluationContext as OFEvaluationContext

        # Register Flagura as OpenFeature provider
        provider = FlaguraOpenFeatureProvider(client=client)
        api.set_provider(provider)

        of_client = api.get_client("analytics-service")
        of_ctx = OFEvaluationContext(
            targeting_key="usr_david_07",
            attributes={
                "email": "david@company.com",
                "tier": "pro",
            }
        )

        bool_res = of_client.get_boolean_value("ai-smart-search", False, of_ctx)
        print(f"OpenFeature get_boolean_value('ai-smart-search'): {bool_res}")

        details_res = of_client.get_string_details("ai-smart-search", "default-v1", of_ctx)
        print(f"OpenFeature get_string_details: Value={details_res.value}, Variant={details_res.variant}, Reason={details_res.reason}")

    except ImportError:
        print("(Note: install 'openfeature-sdk' via pip to run the OpenFeature portion)")

    client.close()
    print("\n✅ Python Native & OpenFeature evaluations completed successfully.")

if __name__ == "__main__":
    main()
