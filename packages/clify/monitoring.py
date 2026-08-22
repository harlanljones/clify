"""ADD Framework - Runtime Monitoring & Observability (§5).

Streams agent telemetry to an in-memory time-series registry and raises
alerts at the thresholds defined in the observability table.
"""

from collections import deque

LATENCY_ALERT_SECONDS = 3.0
COST_ALERT_USD = 0.05
VALIDATION_FAILURE_RATE_ALERT = 0.02
PROVIDER_DEGRADATION_METRIC = "provider_degradation"


class TelemetryRegistry:
    """Central time-series registry for agent telemetry."""

    def __init__(self, latency_window=10):
        self.latency_window = latency_window
        self._latencies = deque(maxlen=latency_window)  # sliding window (10 runs)
        self._cost_total = 0.0
        self._invocations = 0
        self._validation_failures = 0
        self.alerts = []

    def record(self, telemetry: dict) -> list:
        """Record one execution's telemetry; returns any alerts raised."""
        metrics = telemetry["metrics"]
        self._invocations += 1
        self._latencies.append(metrics["duration_seconds"])
        self._cost_total += metrics["calculated_cost_usd"]
        if metrics["confidence_rating"] < 1.0:
            self._validation_failures += 1

        response = telemetry.get("response", {})
        data = response.get("data", {}) if isinstance(response, dict) else {}
        if isinstance(data, list) and len(data) == 1 and isinstance(data[0], dict):
            data = data[0]
        if isinstance(data, dict) and data.get("partial"):
            self.alerts.append({
                "metric": PROVIDER_DEGRADATION_METRIC,
                "sources": list(data.get("failed_sources", [])),
                "action": "Continue with healthy providers and refresh credentials",
            })

        if self.avg_latency > LATENCY_ALERT_SECONDS:
            self.alerts.append({
                "metric": "agent_latency_seconds",
                "action": "Fall back to lightweight prompt template",
            })
        if self.avg_cost > COST_ALERT_USD:
            self.alerts.append({
                "metric": "agent_cost_per_task",
                "action": "Kill runtime engine execution thread",
            })
        if self.validation_failure_rate > VALIDATION_FAILURE_RATE_ALERT:
            self.alerts.append({
                "metric": "validation_failure_rate",
                "action": "Route tasks back to previous safe software build",
            })
        return self.alerts

    @property
    def avg_latency(self) -> float:
        return sum(self._latencies) / len(self._latencies) if self._latencies else 0.0

    @property
    def avg_cost(self) -> float:
        return self._cost_total / self._invocations if self._invocations else 0.0

    @property
    def validation_failure_rate(self) -> float:
        return self._validation_failures / self._invocations if self._invocations else 0.0
