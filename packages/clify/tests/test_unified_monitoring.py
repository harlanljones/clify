from monitoring import PROVIDER_DEGRADATION_METRIC, TelemetryRegistry


def test_partial_provider_result_raises_degradation_alert():
    telemetry = {
        "response": {
            "status": "SUCCESS",
            "data": {
                "partial": True,
                "failed_sources": ["spotify"],
                "recently_played": [],
            },
        },
        "metrics": {
            "duration_seconds": 0.1,
            "calculated_cost_usd": 0.001,
            "confidence_rating": 1.0,
        },
    }
    alerts = TelemetryRegistry().record(telemetry)
    alert = next(a for a in alerts if a["metric"] == PROVIDER_DEGRADATION_METRIC)
    assert alert["sources"] == ["spotify"]
    assert "healthy providers" in alert["action"]


def test_complete_provider_result_does_not_raise_degradation_alert():
    telemetry = {
        "response": {"status": "SUCCESS", "data": {"partial": False}},
        "metrics": {
            "duration_seconds": 0.1,
            "calculated_cost_usd": 0.001,
            "confidence_rating": 1.0,
        },
    }
    alerts = TelemetryRegistry().record(telemetry)
    assert all(a["metric"] != PROVIDER_DEGRADATION_METRIC for a in alerts)
