import unittest
from unittest.mock import patch, MagicMock
import json

from flagura.client import FlaguraClient, EvaluationContext, EvaluationResult

class TestFlaguraPythonSDK(unittest.TestCase):
    def test_evaluation_context(self):
        ctx = EvaluationContext(
            user_id="usr_test_123",
            email="test@example.com",
            country="US",
            role="admin",
            tier="enterprise",
            custom={"beta": True, "score": 95}
        )
        d = ctx.to_dict()
        self.assertEqual(d["user_id"], "usr_test_123")
        self.assertEqual(d["email"], "test@example.com")
        self.assertEqual(d["country"], "US")
        self.assertEqual(d["role"], "admin")
        self.assertEqual(d["tier"], "enterprise")
        self.assertEqual(d["custom"]["beta"], True)
        self.assertEqual(d["custom"]["score"], 95)

    @patch("urllib.request.urlopen")
    def test_evaluate_and_is_enabled(self, mock_urlopen):
        mock_response = MagicMock()
        mock_response.read.return_value = json.dumps({
            "results": {
                "ai-feature": {
                    "flag_key": "ai-feature",
                    "enabled": True,
                    "variant": "on",
                    "value": True,
                    "reason": "PERCENTAGE_ROLLOUT",
                    "latency_ns": 450,
                    "latency_us": 0.45
                }
            }
        }).encode("utf-8")
        mock_response.__enter__.return_value = mock_response
        mock_urlopen.return_value = mock_response

        client = FlaguraClient("http://localhost:8080", api_key="flg_live_secret", project_id="proj_1")
        res = client.evaluate("ai-feature", EvaluationContext(user_id="usr_test_123"))

        self.assertTrue(res.enabled)
        self.assertEqual(res.variant, "on")
        self.assertEqual(res.reason, "PERCENTAGE_ROLLOUT")

        enabled = client.is_enabled("ai-feature", EvaluationContext(user_id="usr_test_123"))
        self.assertTrue(enabled)

    @patch("urllib.request.urlopen")
    def test_track(self, mock_urlopen):
        mock_response = MagicMock()
        mock_response.__enter__.return_value = mock_response
        mock_urlopen.return_value = mock_response

        client = FlaguraClient("http://localhost:8080", api_key="flg_live_secret")
        client.track("ai-feature", "on", "conversion", 1.0, "usr_123")
        self.assertTrue(mock_urlopen.called)

if __name__ == "__main__":
    unittest.main()
