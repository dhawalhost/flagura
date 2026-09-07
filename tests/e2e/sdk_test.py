import os
import sys
from pathlib import Path

# Add sdks/python to path
sdk_dir = str(Path(__file__).resolve().parent.parent.parent / "sdks" / "python")
if sdk_dir not in sys.path:
    sys.path.insert(0, sdk_dir)

from flagura import FlaguraClient, EvaluationContext
# pyrefly: ignore [missing-import]
from flagura.openfeature_provider import FlaguraOpenFeatureProvider

def assert_true(condition, msg):
    if not condition:
        print(f"❌ Assertion failed: {msg}", file=sys.stderr)
        sys.exit(1)

def main():
    endpoint = os.environ.get("FLAGURA_ENDPOINT")
    api_key = os.environ.get("FLAGURA_API_KEY")

    if not endpoint or not api_key:
        print("FLAGURA_ENDPOINT and FLAGURA_API_KEY must be set", file=sys.stderr)
        sys.exit(1)

    print(f"🧪 Running Python SDK E2E Verification against {endpoint}...")

    client = FlaguraClient(
        endpoint=endpoint,
        api_key=api_key,
        default_environment="production",
        enable_streaming=False,
    )

    # 1. Test Boolean Flag
    user_ctx = EvaluationContext(user_id="usr_py_1")
    bool_res = client.evaluate("e2e-bool-active", user_ctx)
    print(f"  ✓ Boolean Flag (e2e-bool-active): enabled={bool_res.enabled}, reason={bool_res.reason}")
    assert_true(bool_res.enabled is True, "e2e-bool-active should be enabled")

    # 2. Test Percentage Rollout Flag (Deterministic hashing)
    user_a = client.evaluate("e2e-percentage-rollout", EvaluationContext(user_id="usr_fixed_hash_1"))
    user_b = client.evaluate("e2e-percentage-rollout", EvaluationContext(user_id="usr_fixed_hash_1"))
    assert_true(user_a.enabled == user_b.enabled, "Rollout evaluation must be deterministic")
    print(f"  ✓ Deterministic Rollout (e2e-percentage-rollout): user_fixed_hash_1={user_a.enabled} (consistent)")

    # 3. Test Rule Targeting Flag
    dev_ctx = EvaluationContext(user_id="usr_dev_1", role="developer", email="dev@flagura.dev")
    guest_ctx = EvaluationContext(user_id="usr_guest_1", role="guest", email="guest@flagura.dev")

    dev_res = client.evaluate("e2e-rule-targeting", dev_ctx)
    guest_res = client.evaluate("e2e-rule-targeting", guest_ctx)
    print(f"  ✓ Rule Targeting (e2e-rule-targeting): dev={dev_res.enabled}, guest={guest_res.enabled}")
    assert_true(dev_res.enabled is True, "Developer role should match targeting rule and be enabled")
    assert_true(guest_res.enabled is False, "Guest role should not match targeting rule and be disabled")

    # 4. Test Multivariate Flag
    mv_ctx = EvaluationContext(user_id="usr_ai_model_test")
    mv_res = client.evaluate("e2e-multivariate-models", mv_ctx)
    print(f"  ✓ Multivariate Flag (e2e-multivariate-models): variant={mv_res.variant}")
    assert_true(bool(mv_res.variant), "Multivariate variant must not be empty")

    # 5. OpenFeature Provider Verification
    provider = FlaguraOpenFeatureProvider(client=client)
    res_bool = provider.resolve_boolean_details("e2e-bool-active", False, {"user_id": "usr_py_1"})
    assert_true(res_bool.value is True, "OpenFeature provider resolve_boolean_details must return True")
    res_str = provider.resolve_string_details("e2e-multivariate-models", "default", {"user_id": "usr_ai_model_test"})
    print(f"  ✓ OpenFeature Provider: boolean={res_bool.value}, stringVariant={res_str.variant}")

    # 6. Telemetry Event Ingestion
    client.track("e2e-bool-active", "treatment", "e2e_checkout", 49.99, "usr_py_1")
    print("  ✓ Telemetry Event: track() executed successfully")

    client.close()
    print("🎉 Python SDK E2E Verification PASSED!\n")

if __name__ == "__main__":
    main()
