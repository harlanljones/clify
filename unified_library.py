"""Provider-independent library aggregation (ROADMAP Phase 7)."""

from __future__ import annotations

from datetime import datetime, timezone
import json
import os
from typing import Any, Iterable, Mapping, Sequence

from core_agents import BaseAgent, ScopeGuard, ToolError


_MANIFEST_PATH = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "agent_manifest.cliamp_library.json"
)
_DEFAULT_SOURCE_NAMES = ("cliamp", "spotify")
_HISTORY_TERMS = {"history", "recently", "played"}
_LIBRARY_TERMS = {"library", "playlist", "playlists"}


def load_manifest(path: str | None = None) -> dict:
    """Load the unified library agent's scope contract."""
    with open(path or _MANIFEST_PATH, "r", encoding="utf-8") as stream:
        return json.load(stream)


def _source_pairs(sources: Any) -> Iterable[tuple[str, Sequence[dict]]]:
    if isinstance(sources, Mapping):
        yield from sources.items()
        return
    for index, source in enumerate(sources):
        if (isinstance(source, tuple) and len(source) == 2
                and isinstance(source[0], str)):
            yield source[0], source[1]
        else:
            name = (_DEFAULT_SOURCE_NAMES[index] if index < len(_DEFAULT_SOURCE_NAMES)
                    else f"provider_{index + 1}")
            yield name, source


def _timestamp(value: Any) -> datetime:
    if not isinstance(value, str) or not value:
        return datetime.min.replace(tzinfo=timezone.utc)
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.astimezone(timezone.utc)
    except ValueError:
        return datetime.min.replace(tzinfo=timezone.utc)


def _track_identity(item: Mapping[str, Any]) -> tuple:
    track = item.get("track") if isinstance(item.get("track"), Mapping) else item
    title = track.get("title") or track.get("name")
    artists = track.get("artists") or track.get("artist") or item.get("artist")
    if isinstance(artists, list):
        artists = ",".join(
            str(artist.get("name", "") if isinstance(artist, Mapping) else artist)
            for artist in artists
        )
    if title:
        return ("track", str(title).strip().casefold(), str(artists or "").strip().casefold())
    location = track.get("uri") or track.get("path") or item.get("uri") or item.get("path")
    if location:
        return ("location", str(location).strip().casefold())
    # Unknown-shaped entries should not accidentally collapse into one.
    return ("raw", repr(sorted(item.items(), key=lambda pair: str(pair[0]))))


def merge_recently_played(sources: list[list[dict]]) -> list[dict]:
    """Merge history sources, source-tag, deduplicate, and newest-sort.

    The documented list-of-lists input convention names its first two
    sources ``cliamp`` and ``spotify``. Mappings or ``(name, entries)`` pairs
    may be supplied when callers configure additional providers.
    """
    merged: dict[tuple, dict] = {}
    provenance: dict[tuple, list[str]] = {}
    for source_name, entries in _source_pairs(sources):
        for raw in entries or []:
            if not isinstance(raw, Mapping):
                continue
            item = dict(raw)
            item["source"] = source_name
            identity = _track_identity(item)
            source_list = provenance.setdefault(identity, [])
            if source_name not in source_list:
                source_list.append(source_name)
            current = merged.get(identity)
            if current is None or _timestamp(item.get("played_at")) > _timestamp(
                    current.get("played_at")):
                merged[identity] = item

    for identity, item in merged.items():
        if len(provenance[identity]) > 1:
            item["sources"] = list(provenance[identity])
    return sorted(
        merged.values(), key=lambda item: _timestamp(item.get("played_at")), reverse=True
    )


class UnifiedLibraryClient:
    """Aggregate read APIs while isolating each provider's failures."""

    def __init__(self, cliamp_client: Any, spotify_client: Any):
        self.cliamp = cliamp_client
        self.spotify = spotify_client
        self.failed_sources: list[str] = []

    @staticmethod
    def _history_items(payload: Any, key: str) -> list[dict]:
        if isinstance(payload, Mapping):
            payload = payload.get(key, [])
        return list(payload) if isinstance(payload, (list, tuple)) else []

    def _recent(self, limit: int) -> tuple[list[dict], list[str]]:
        if limit < 0:
            raise ValueError("limit must be non-negative")
        unified_reader = getattr(self.cliamp, "get_unified_recently_played", None)
        if callable(unified_reader):
            try:
                payload = unified_reader(limit=limit)
                if (isinstance(payload, Mapping)
                        and payload.get("schema_version") == "cliamp.history.unified/1"
                        and isinstance(payload.get("history"), list)):
                    failures = payload.get("failed_sources", [])
                    return list(payload["history"])[:limit], list(failures or [])
            except Exception:
                # Stock cliamp has no history.unified command. Capability
                # absence must remain a transparent fallback, not a failure.
                pass
        sources = []
        failures = []
        for name, provider, key in (
            ("cliamp", self.cliamp, "entries"),
            ("spotify", self.spotify, "items"),
        ):
            try:
                payload = provider.get_recently_played(limit=limit)
                sources.append((name, self._history_items(payload, key)))
            except Exception:
                failures.append(name)
        return merge_recently_played(sources)[:limit], failures

    def get_recently_played(self, limit: int = 20) -> list[dict]:
        items, self.failed_sources = self._recent(limit)
        return items

    def get_library_sections(self) -> dict:
        recent, failures = self._recent(20)
        try:
            payload = self.cliamp.get_playlists()
            library = payload.get("playlists", []) if isinstance(payload, Mapping) else payload
            library = list(library or [])
        except Exception:
            library = []
            failures.append("cliamp")
        try:
            payload = self.spotify.get_user_playlists()
            playlists = payload.get("items", []) if isinstance(payload, Mapping) else payload
            playlists = list(playlists or [])
        except Exception:
            playlists = []
            failures.append("spotify")
        self.failed_sources = list(dict.fromkeys(failures))
        return {
            "recently_played": recent,
            "library": library,
            "your_playlists": playlists,
            "partial": bool(self.failed_sources),
            "failed_sources": list(self.failed_sources),
        }


class UnifiedLibraryAgent(BaseAgent):
    """Read-only BaseAgent facade over :class:`UnifiedLibraryClient`."""

    def __init__(self, identity="cliamp_library_agent", tools=None,
                 manifest=None, manifest_path=None):
        manifest = manifest if manifest is not None else load_manifest(manifest_path)
        if tools is None:
            raise ValueError("CliampClient and SpotifyClient tools must be injected")
        if len(tools) != 2:
            raise ValueError("expected [CliampClient, SpotifyClient]")
        super().__init__(
            identity=identity,
            tools=tools,
            configured_scopes=manifest["allowed_scopes"],
            prohibited_scopes=manifest["prohibited_scopes"],
            manifest=manifest,
        )
        self.client = UnifiedLibraryClient(tools[0], tools[1])

    def is_task_authorized(self, task: str) -> bool:
        """Avoid diluting short, recognized queries with filler words."""
        tokens = ScopeGuard._tokenize(task)
        if not tokens & (_HISTORY_TERMS | _LIBRARY_TERMS):
            return False
        return self.guard.is_authorized(
            task, similarity_text=self.manifest["description"]
        )

    def _route_tool(self, instruction: str):
        tokens = ScopeGuard._tokenize(instruction)
        if tokens & _HISTORY_TERMS:
            return self.client.get_recently_played()
        if tokens & _LIBRARY_TERMS:
            return self.client.get_library_sections()
        raise ToolError(f"No tool route matched instruction: {instruction!r}")
