from typing import Any, Dict, Optional
from .client import FlaguraClient, EvaluationContext as FlaguraContext


class ResolutionDetails:
    """Represents flag resolution details conforming to OpenFeature spec."""

    def __init__(
        self,
        value: Any,
        variant: Optional[str] = None,
        reason: Optional[str] = None,
        error_code: Optional[str] = None,
        error_message: Optional[str] = None,
    ):
        self.value = value
        self.variant = variant
        self.reason = reason
        self.error_code = error_code
        self.error_message = error_message

    def __repr__(self) -> str:
        return f"ResolutionDetails(value={self.value!r}, variant={self.variant!r}, reason={self.reason!r})"


class FlaguraOpenFeatureProvider:
    """Official Flagura OpenFeature Provider for Python applications."""

    def __init__(self, client: Optional[FlaguraClient] = None, **kwargs: Any):
        if client is not None:
            self.client = client
        else:
            self.client = FlaguraClient(**kwargs)
        self.name = "flagura-python-provider"

    def get_metadata(self) -> Dict[str, str]:
        return {"name": self.name}

    def _map_context(self, context: Any) -> FlaguraContext:
        if context is None:
            return FlaguraContext(user_id="anonymous")

        # OpenFeature evaluation context in Python can be an object with targeting_key and attributes dict
        user_id = getattr(context, "targeting_key", None)
        attributes = getattr(context, "attributes", {})

        if isinstance(context, dict):
            user_id = context.get("targeting_key") or context.get("user_id") or context.get("userId")
            attributes = context

        if not user_id:
            user_id = attributes.get("user_id") or attributes.get("userId") or "anonymous"

        return FlaguraContext(
            user_id=str(user_id),
            email=attributes.get("email"),
            country=attributes.get("country"),
            role=attributes.get("role"),
            tier=attributes.get("tier"),
            environment=attributes.get("environment", self.client.default_environment),
            custom={
                k: v
                for k, v in attributes.items()
                if k not in {"targeting_key", "user_id", "userId", "email", "country", "role", "tier", "environment"}
            },
        )

    def _map_reason(self, reason: str) -> str:
        r = (reason or "").upper()
        if "TARGETING" in r or "RULE" in r:
            return "TARGETING_MATCH"
        if "PERCENTAGE" in r or "MULTIVARIATE" in r or "BUCKET" in r:
            return "SPLIT"
        if "KILL_SWITCH" in r or "ENV_DISABLED" in r or "DISABLED" in r:
            return "DISABLED"
        if "DEFAULT" in r:
            return "DEFAULT"
        return "STATIC"

    def resolve_boolean_details(
        self, flag_key: str, default_value: bool, evaluation_context: Any = None
    ) -> ResolutionDetails:
        try:
            ctx = self._map_context(evaluation_context)
            res = self.client.evaluate(flag_key, ctx)

            if res.reason == "FLAG_NOT_FOUND":
                return ResolutionDetails(
                    value=default_value,
                    reason="ERROR",
                    error_code="FLAG_NOT_FOUND",
                    error_message=f"Flag '{flag_key}' not found",
                )

            if not res.enabled:
                return ResolutionDetails(
                    value=default_value,
                    variant=res.variant or "off",
                    reason=self._map_reason(res.reason),
                )

            val = res.value if isinstance(res.value, bool) else bool(res.value) if res.value is not None else True
            return ResolutionDetails(
                value=val,
                variant=res.variant or "treatment",
                reason=self._map_reason(res.reason),
            )
        except Exception as e:
            return ResolutionDetails(
                value=default_value,
                reason="ERROR",
                error_code="GENERAL",
                error_message=str(e),
            )

    def resolve_string_details(
        self, flag_key: str, default_value: str, evaluation_context: Any = None
    ) -> ResolutionDetails:
        try:
            ctx = self._map_context(evaluation_context)
            res = self.client.evaluate(flag_key, ctx)

            if res.reason == "FLAG_NOT_FOUND":
                return ResolutionDetails(
                    value=default_value,
                    reason="ERROR",
                    error_code="FLAG_NOT_FOUND",
                    error_message=f"Flag '{flag_key}' not found",
                )

            if not res.enabled:
                return ResolutionDetails(
                    value=default_value,
                    variant=res.variant or "off",
                    reason=self._map_reason(res.reason),
                )

            val = str(res.value) if res.value else res.variant or default_value
            return ResolutionDetails(
                value=val,
                variant=res.variant or "treatment",
                reason=self._map_reason(res.reason),
            )
        except Exception as e:
            return ResolutionDetails(
                value=default_value,
                reason="ERROR",
                error_code="GENERAL",
                error_message=str(e),
            )

    def resolve_integer_details(
        self, flag_key: str, default_value: int, evaluation_context: Any = None
    ) -> ResolutionDetails:
        try:
            ctx = self._map_context(evaluation_context)
            res = self.client.evaluate(flag_key, ctx)

            if res.reason == "FLAG_NOT_FOUND":
                return ResolutionDetails(
                    value=default_value,
                    reason="ERROR",
                    error_code="FLAG_NOT_FOUND",
                    error_message=f"Flag '{flag_key}' not found",
                )

            if not res.enabled:
                return ResolutionDetails(
                    value=default_value,
                    variant=res.variant or "off",
                    reason=self._map_reason(res.reason),
                )

            try:
                val = int(res.value)
            except (ValueError, TypeError):
                val = default_value

            return ResolutionDetails(
                value=val,
                variant=res.variant or "treatment",
                reason=self._map_reason(res.reason),
            )
        except Exception as e:
            return ResolutionDetails(
                value=default_value,
                reason="ERROR",
                error_code="GENERAL",
                error_message=str(e),
            )

    def resolve_float_details(
        self, flag_key: str, default_value: float, evaluation_context: Any = None
    ) -> ResolutionDetails:
        try:
            ctx = self._map_context(evaluation_context)
            res = self.client.evaluate(flag_key, ctx)

            if res.reason == "FLAG_NOT_FOUND":
                return ResolutionDetails(
                    value=default_value,
                    reason="ERROR",
                    error_code="FLAG_NOT_FOUND",
                    error_message=f"Flag '{flag_key}' not found",
                )

            if not res.enabled:
                return ResolutionDetails(
                    value=default_value,
                    variant=res.variant or "off",
                    reason=self._map_reason(res.reason),
                )

            try:
                val = float(res.value)
            except (ValueError, TypeError):
                val = default_value

            return ResolutionDetails(
                value=val,
                variant=res.variant or "treatment",
                reason=self._map_reason(res.reason),
            )
        except Exception as e:
            return ResolutionDetails(
                value=default_value,
                reason="ERROR",
                error_code="GENERAL",
                error_message=str(e),
            )

    def resolve_object_details(
        self, flag_key: str, default_value: Any, evaluation_context: Any = None
    ) -> ResolutionDetails:
        try:
            ctx = self._map_context(evaluation_context)
            res = self.client.evaluate(flag_key, ctx)

            if res.reason == "FLAG_NOT_FOUND":
                return ResolutionDetails(
                    value=default_value,
                    reason="ERROR",
                    error_code="FLAG_NOT_FOUND",
                    error_message=f"Flag '{flag_key}' not found",
                )

            if not res.enabled:
                return ResolutionDetails(
                    value=default_value,
                    variant=res.variant or "off",
                    reason=self._map_reason(res.reason),
                )

            val = res.value if res.value is not None else default_value
            return ResolutionDetails(
                value=val,
                variant=res.variant or "treatment",
                reason=self._map_reason(res.reason),
            )
        except Exception as e:
            return ResolutionDetails(
                value=default_value,
                reason="ERROR",
                error_code="GENERAL",
                error_message=str(e),
            )
