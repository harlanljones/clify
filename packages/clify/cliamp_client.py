"""CliampClient — thin, injectable subprocess wrapper around the cliamp binary.

Single seam for all framework -> cliamp interaction (ROADMAP Phase 1).
Transport: subprocess + ``--json`` (stateless), per the Phase 0 decision and
docs/cliamp_schemas.md. Every call is::

    subprocess.run(argv, capture_output=True, text=True, timeout=2.0)

The 2.0s timeout keeps the AGENTS.md §3.1 latency SLA (<= 2.5s) achievable.

All failures raise ToolError-compatible exceptions carrying a structured
payload of the form ``{"status": "TOOL_ERROR", "retry_allowed": ...}``
(AGENTS.md §4.2).
"""

import json
import subprocess
from dataclasses import dataclass, field
from typing import Any, Optional

CLIAMP_BINARY = "cliamp"
SUBPROCESS_TIMEOUT = 2.0


class ToolError(Exception):
    """Tool failure carrying the AGENTS.md §4.2 structured error payload."""

    def __init__(self, message: str, retry_allowed: bool = True):
        super().__init__(message)
        self.message = message
        self.retry_allowed = retry_allowed

    @property
    def payload(self) -> dict:
        return {
            "status": "TOOL_ERROR",
            "retry_allowed": self.retry_allowed,
            "error": self.message,
        }


class BinaryMissingError(ToolError):
    """cliamp binary not found on PATH (FileNotFoundError). Not retryable."""

    def __init__(self, binary: str):
        super().__init__(
            f"cliamp binary not found on PATH: {binary!r}", retry_allowed=False
        )


class CliampCommandError(ToolError):
    """cliamp exited non-zero. ``retry_allowed=False`` for unknown resources."""


class CliampTimeoutError(ToolError):
    """Subprocess exceeded the 2.0s timeout. Retryable."""

    def __init__(self, argv: list):
        super().__init__(
            f"cliamp timed out after {SUBPROCESS_TIMEOUT}s: {argv!r}",
            retry_allowed=True,
        )


class CliampParseError(ToolError):
    """Empty stdout or malformed JSON where JSON was expected. Retryable."""


@dataclass
class Track:
    """One playlist entry (cliamp.playlist.show/1). Fields are pass-through."""

    path: str
    title: Optional[str] = None
    duration: Optional[float] = None
    bookmarked: Optional[bool] = None

    @classmethod
    def from_dict(cls, raw: dict) -> "Track":
        return cls(
            path=raw.get("path", ""),
            title=raw.get("title"),
            duration=raw.get("duration"),
            bookmarked=raw.get("bookmarked"),
        )


@dataclass
class Playlist:
    """Track listing of one playlist (cliamp.playlist.show/1)."""

    name: str
    tracks: list = field(default_factory=list)

    def to_dict(self) -> dict:
        return {"name": self.name, "tracks": [vars(t) for t in self.tracks]}


@dataclass
class HistoryEntry:
    """One listening-history entry (cliamp.history/1)."""

    title: str
    path: str
    played_at: Optional[str] = None
    stream: Optional[bool] = None

    @classmethod
    def from_dict(cls, raw: dict) -> "HistoryEntry":
        return cls(
            title=raw.get("title", ""),
            path=raw.get("path", ""),
            played_at=raw.get("played_at"),
            stream=raw.get("stream"),
        )


@dataclass
class PlayerStatus:
    """Player state snapshot (cliamp.status/1). Extra fields pass through."""

    ok: bool
    state: str
    track: Optional[dict] = None
    extra: dict = field(default_factory=dict)

    @classmethod
    def from_dict(cls, raw: dict) -> "PlayerStatus":
        known = {"ok", "state", "track"}
        return cls(
            ok=bool(raw.get("ok", False)),
            state=raw.get("state", "stopped"),
            track=raw.get("track"),
            extra={k: v for k, v in raw.items() if k not in known},
        )


class CliampClient:
    """Read-only wrapper around cliamp's machine-readable subcommands."""

    def __init__(self, binary: str = CLIAMP_BINARY):
        self._binary = binary

    # -- subprocess boundary -------------------------------------------------

    def _run(self, argv: list) -> "subprocess.CompletedProcess":
        full_argv = [self._binary] + argv
        try:
            proc = subprocess.run(
                full_argv,
                capture_output=True,
                text=True,
                timeout=SUBPROCESS_TIMEOUT,
            )
        except FileNotFoundError:
            raise BinaryMissingError(self._binary)
        except subprocess.TimeoutExpired:
            raise CliampTimeoutError(full_argv)
        if proc.returncode != 0:
            stderr = (proc.stderr or "").strip()
            retry = "not found" not in stderr
            raise CliampCommandError(
                f"cliamp exited {proc.returncode}: {stderr or proc.stdout.strip()}",
                retry_allowed=retry,
            )
        return proc

    def _run_json(self, argv: list) -> Any:
        proc = self._run(argv)
        stdout = (proc.stdout or "").strip()
        if not stdout:
            raise CliampParseError(
                f"cliamp returned empty stdout where JSON was expected: {argv!r}"
            )
        try:
            return json.loads(stdout)
        except json.JSONDecodeError as exc:
            raise CliampParseError(f"malformed JSON from cliamp: {exc}")

    # -- read API ------------------------------------------------------------

    def get_playlists(self) -> dict:
        """cliamp playlist list — no --json exists; parse the text output."""
        proc = self._run(["playlist", "list"])
        names = []
        for line in (proc.stdout or "").splitlines():
            line = line.strip()
            if not line or line == "No playlists found.":
                continue
            # Lines are "Name (N tracks)"; take everything before " (".
            name = line.rsplit(" (", 1)[0] if " (" in line else line
            names.append(name)
        return {"schema_version": "cliamp.playlist.list/1", "playlists": names}

    def get_playlist(self, name: str) -> dict:
        """cliamp playlist show NAME --json — bare JSON array of tracks."""
        if not name or not name.strip():
            raise CliampCommandError(
                "playlist name must be a non-empty string", retry_allowed=False
            )
        payload = self._run_json(["playlist", "show", name, "--json"])
        if not isinstance(payload, list):
            raise CliampParseError(
                f"expected a JSON array of tracks, got {type(payload).__name__}"
            )
        playlist = Playlist(name=name, tracks=[Track.from_dict(t) for t in payload])
        return playlist.to_dict()

    def get_recently_played(self, limit: int = 50) -> dict:
        """cliamp history --json --limit N — JSON array of entries."""
        payload = self._run_json(["history", "--json", "--limit", str(limit)])
        if not isinstance(payload, list):
            raise CliampParseError(
                f"expected a JSON array of history entries, got {type(payload).__name__}"
            )
        entries = [HistoryEntry.from_dict(e) for e in payload]
        return {
            "schema_version": "cliamp.history/1",
            "entries": [vars(e) for e in entries],
        }

    def get_unified_recently_played(self, limit: int = 20) -> dict:
        """Read the fork-only ``cliamp.history.unified/1`` IPC contract.

        Stock cliamp rejects this subcommand; callers should treat that command
        error as a capability miss and retain their existing merge fallback.
        """
        payload = self._run_json(
            ["history", "unified", "--json", "--limit", str(limit)]
        )
        if not isinstance(payload, dict):
            raise CliampParseError(
                f"expected a unified-history JSON object, got {type(payload).__name__}"
            )
        if payload.get("schema_version") != "cliamp.history.unified/1":
            raise CliampParseError(
                "unsupported unified-history schema: "
                f"{payload.get('schema_version')!r}"
            )
        if not isinstance(payload.get("history"), list):
            raise CliampParseError("unified-history response is missing history array")
        return payload

    def get_status(self) -> dict:
        """cliamp status --json — player state object."""
        payload = self._run_json(["status", "--json"])
        if not isinstance(payload, dict):
            raise CliampParseError(
                f"expected a JSON object for status, got {type(payload).__name__}"
            )
        status = PlayerStatus.from_dict(payload)
        result = {
            "schema_version": "cliamp.status/1",
            "ok": status.ok,
            "state": status.state,
            "track": status.track,
        }
        result.update(status.extra)
        return result
