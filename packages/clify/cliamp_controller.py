"""CliampController — mutation-capable tool wrapping cliamp IPC verbs.

ROADMAP Phase 3. One of two agents/tools in the framework allowed to mutate
player state. Verbs: play, pause, toggle, stop, next, prev, volume(db),
seek(seconds), queue(path), load(playlist), shuffle, repeat.

Transport: ``subprocess.run(argv, capture_output=True, timeout=2.0)`` per the
Phase 0 decision (``SUBPROCESS_TIMEOUT`` from cliamp_client).

Daemon precondition (fail-fast, no auto-spawn): if ``cliamp status`` fails or
reports ``ok == False``, every mutation raises
:class:`CliampDaemonError` with ``retry_allowed=False`` — the daemon is
operator-managed (ROADMAP Phase 0 / docs/cliamp_schemas.md §5).

Every verb returns the post-command ``cliamp status --json`` payload so the
agent verification loop can confirm the resulting state.
"""

import subprocess

from cliamp_client import (
    CLIAMP_BINARY,
    SUBPROCESS_TIMEOUT,
    BinaryMissingError,
    CliampClient,
    CliampCommandError,
    CliampTimeoutError,
    ToolError,
)


class CliampDaemonError(ToolError):
    """No running cliamp instance to mutate. Fail-fast, not retryable."""

    def __init__(self, message: str):
        super().__init__(message, retry_allowed=False)


class CliampController:
    """Playback-control seam for cliamp_playback_agent (and nothing else)."""

    def __init__(self, binary: str = CLIAMP_BINARY):
        self._binary = binary
        self._reader = CliampClient(binary)  # status reads reuse the Phase 1 seam

    # -- subprocess boundary -------------------------------------------------

    def _run(self, argv: list):
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
            # A failed control verb signals daemon-down: fail-fast.
            raise CliampDaemonError(
                f"cliamp control command failed (exit {proc.returncode}): "
                f"{stderr or (proc.stdout or '').strip() or '<no output>'} "
                f"[argv={full_argv!r}]"
            )
        return proc

    def _ensure_daemon(self) -> None:
        """Precondition: a running instance must answer ``status --json``."""
        try:
            payload = self._reader._run_json(["status", "--json"])
        except CliampCommandError as exc:
            raise CliampDaemonError(
                f"cliamp status failed (daemon down): {exc.message}"
            )
        if not isinstance(payload, dict) or payload.get("ok") is False:
            raise CliampDaemonError(
                "cliamp status reports ok=False; no running daemon to mutate"
            )

    def _mutate(self, argv: list) -> dict:
        """Run a control verb, then return post-command status for verification."""
        self._ensure_daemon()
        self._run(argv)
        return self._reader.get_status()

    # -- IPC verbs (playback.control scope) ----------------------------------

    def play(self) -> dict:
        return self._mutate(["play"])

    def pause(self) -> dict:
        return self._mutate(["pause"])

    def toggle(self) -> dict:
        return self._mutate(["toggle"])

    def stop(self) -> dict:
        return self._mutate(["stop"])

    def next(self) -> dict:
        return self._mutate(["next"])

    def prev(self) -> dict:
        return self._mutate(["prev"])

    def volume(self, db) -> dict:
        return self._mutate(["volume", str(db)])

    def seek(self, seconds) -> dict:
        return self._mutate(["seek", str(seconds)])

    def queue(self, path) -> dict:
        return self._mutate(["queue", str(path)])

    def load(self, playlist) -> dict:
        return self._mutate(["load", str(playlist)])

    def shuffle(self) -> dict:
        return self._mutate(["shuffle"])

    def repeat(self) -> dict:
        return self._mutate(["repeat"])
