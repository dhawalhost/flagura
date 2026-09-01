from dataclasses import dataclass, field
from typing import Any, Callable, Dict, List, Optional
import urllib.request
import urllib.error
import json
import threading
import time
import datetime


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
        project_id: Optional[str] = None,
        default_environment: str = "production",
        timeout: float = 5.0,
        enable_streaming: bool = False,
    ):
        self.endpoint = endpoint.rstrip("/")
        self.api_key = api_key
        self.project_id = project_id
        self.default_environment = default_environment
        self.timeout = timeout
        self._local_flags: Dict[str, Any] = {}
        self._listeners: List[Callable[[Dict[str, Any]], None]] = []
        self._stop_event = threading.Event()
        self._stream_thread: Optional[threading.Thread] = None

        if enable_streaming:
            self._start_streaming()

    def on_update(self, callback: Callable[[Dict[str, Any]], None]) -> None:
        """Register a callback invoked when feature flags are updated in real time."""
        self._listeners.append(callback)

    def _start_streaming(self) -> None:
        self._stream_thread = threading.Thread(target=self._stream_worker, daemon=True)
        self._stream_thread.start()

    def _stream_worker(self) -> None:
        url = f"{self.endpoint}/api/v1/flags/stream"
        headers = {"Accept": "text/event-stream"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        if self.project_id:
            headers["X-Project-ID"] = self.project_id

        while not self._stop_event.is_set():
            try:
                req = urllib.request.Request(url, headers=headers)
                with urllib.request.urlopen(req, timeout=30) as resp:
                    for line_bytes in resp:
                        if self._stop_event.is_set():
                            break
                        line = line_bytes.decode("utf-8").strip()
                        if line.startswith("data:"):
                            try:
                                data_str = line[5:].strip()
                                if data_str == "ping" or not data_str:
                                    continue
                                payload = json.loads(data_str)
                                if isinstance(payload, list):
                                    self._local_flags = {f["key"]: f for f in payload if "key" in f}
                                elif isinstance(payload, dict) and "flags" in payload:
                                    raw_flags = payload["flags"]
                                    if isinstance(raw_flags, list):
                                        self._local_flags = {f["key"]: f for f in raw_flags if "key" in f}
                                    elif isinstance(raw_flags, dict):
                                        self._local_flags = raw_flags
                                for listener in self._listeners:
                                    listener(dict(self._local_flags))
                            except Exception:
                                pass
            except Exception:
                if not self._stop_event.is_set():
                    time.sleep(3.0)

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

        req_data = json.dumps(payload).encode("utf-8")
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        if self.project_id:
            headers["X-Project-ID"] = self.project_id

        req = urllib.request.Request(
            f"{self.endpoint}/api/v1/evaluate",
            data=req_data,
            headers=headers,
            method="POST",
        )

        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                results: Dict[str, EvaluationResult] = {}
                raw_results = data.get("results", {})

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
        except Exception as e:
            raise RuntimeError(f"Flagura evaluation request failed: {e}") from e

    def track(self, flag_key: str, variant: str, metric_name: str, value: float = 1.0, user_id: str = "") -> None:
        """Track an experiment conversion or numeric metric event."""
        payload = {
            "events": [
                {
                    "flag_key": flag_key,
                    "project_id": self.project_id,
                    "variant": variant,
                    "metric_name": metric_name,
                    "value": value,
                    "user_id": user_id,
                    "environment": self.default_environment,
                    "timestamp": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                }
            ]
        }
        req_data = json.dumps(payload).encode("utf-8")
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        if self.project_id:
            headers["X-Project-ID"] = self.project_id

        req = urllib.request.Request(
            f"{self.endpoint}/api/v1/telemetry/events",
            data=req_data,
            headers=headers,
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout):
                pass
        except Exception:
            pass

    def close(self) -> None:
        """Close active background stream threads."""
        self._stop_event.set()
