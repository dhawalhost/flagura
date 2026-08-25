from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional
import urllib.request
import urllib.error
import json


@dataclass
class EvaluationContext:
    user_id: str
    email: Optional[str] = None
    country: Optional[str] = None
    role: Optional[str] = None
    tier: Optional[str] = None
    environment: str = "production"
    custom: Dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> Dict[str, Any]:
        d: Dict[str, Any] = {
            "user_id": self.user_id,
            "environment": self.environment,
        }
        if self.email:
            d["email"] = self.email
        if self.country:
            d["country"] = self.country
        if self.role:
            d["role"] = self.role
        if self.tier:
            d["tier"] = self.tier
        if self.custom:
            d["custom"] = self.custom
        return d


@dataclass
class EvaluationResult:
    flag_key: str
    enabled: bool
    variant: str = "off"
    value: Any = False
    reason: str = ""
    bucket: Optional[float] = None
    latency_ns: int = 0
    latency_us: float = 0.0


class FlaguraClient:
    """Official Python Client for Flagura Feature Flag Platform."""

    def __init__(
        self,
        endpoint: str = "http://localhost:3000",
        api_key: Optional[str] = None,
        default_environment: str = "production",
        timeout: float = 5.0,
    ):
        self.endpoint = endpoint.rstrip("/")
        self.api_key = api_key
        self.default_environment = default_environment
        self.timeout = timeout

    def is_enabled(self, flag_key: str, context: EvaluationContext) -> bool:
        """Check if a boolean flag is enabled for the given context."""
        try:
            res = self.evaluate(flag_key, context)
            return bool(res.enabled)
        except Exception:
            return False

    def get_variant(self, flag_key: str, context: EvaluationContext, fallback: str = "off") -> str:
        """Retrieve the assigned variant string for a multivariate flag."""
        try:
            res = self.evaluate(flag_key, context)
            return res.variant if res.variant and res.variant != "off" else fallback
        except Exception:
            return fallback

    def evaluate(self, flag_key: str, context: EvaluationContext) -> EvaluationResult:
        """Evaluate a single feature flag."""
        results = self.evaluate_batch([flag_key], context)
        if flag_key in results:
            return results[flag_key]
        return EvaluationResult(
            flag_key=flag_key,
            enabled=False,
            variant="off",
            value=False,
            reason="FLAG_NOT_FOUND",
        )

    def evaluate_batch(self, flag_keys: List[str], context: EvaluationContext) -> Dict[str, EvaluationResult]:
        """Evaluate multiple feature flags concurrently in a single HTTP payload."""
        ctx_dict = context.to_dict()
        if "environment" not in ctx_dict or not ctx_dict["environment"]:
            ctx_dict["environment"] = self.default_environment

        payload = {
            "flags": flag_keys,
            "context": ctx_dict,
        }

        url = f"{self.endpoint}/api/v1/evaluate"
        headers = {
            "Content-Type": "application/json",
            "User-Agent": "flagura-python-sdk/1.0.0",
        }
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"

        data = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(url, data=data, headers=headers, method="POST")

        with urllib.request.urlopen(req, timeout=self.timeout) as resp:
            if resp.status != 200:
                raise RuntimeError(f"Evaluation returned HTTP status {resp.status}")
            resp_body = resp.read().decode("utf-8")
            body_json = json.loads(resp_body)

        raw_results = body_json.get("results", {})
        results: Dict[str, EvaluationResult] = {}

        for k, v in raw_results.items():
            results[k] = EvaluationResult(
                flag_key=v.get("flag_key", k),
                enabled=v.get("enabled", False),
                variant=v.get("variant", "off"),
                value=v.get("value", False),
                reason=v.get("reason", ""),
                bucket=v.get("bucket"),
                latency_ns=v.get("latency_ns", 0),
                latency_us=v.get("latency_us", 0.0),
            )

        return results
