"""Command-line entrypoint for clify's unified music library."""

import argparse
import json
import sys


SECTION_LABELS = (
    ("recently_played", "Recently Played"),
    ("library", "Library"),
    ("your_playlists", "Your Playlists"),
)


def _parser():
    parser = argparse.ArgumentParser(
        prog="clify", description="Query and control your unified music library."
    )
    commands = parser.add_subparsers(dest="command", required=True)

    library = commands.add_parser("library", help="show the unified library")
    library.add_argument("--json", action="store_true", dest="as_json")

    recent = commands.add_parser("recent", help="show merged listening history")
    recent.add_argument("--limit", type=int, default=20)
    recent.add_argument("--json", action="store_true", dest="as_json")

    play = commands.add_parser("play", help="send one playback instruction")
    play.add_argument("instruction", nargs="+")
    play.add_argument("--json", action="store_true", dest="as_json")

    status = commands.add_parser("status", help="show cliamp player status")
    status.add_argument("--json", action="store_true", dest="as_json")

    spotify = commands.add_parser("spotify", help="configure Spotify access")
    spotify_commands = spotify.add_subparsers(dest="spotify_command", required=True)
    spotify_login_parser = spotify_commands.add_parser(
        "login", help="authorize with Spotify using PKCE"
    )
    spotify_login_parser.add_argument(
        "--client-id", help="public Spotify application Client ID"
    )
    return parser


def _default_unified_client():
    from cliamp_client import CliampClient
    from spotify_client import SpotifyClient
    from unified_library import UnifiedLibraryClient

    # SpotifyClient reads its OAuth configuration from the documented
    # SPOTIFY_* environment variables when explicit values are omitted.
    return UnifiedLibraryClient(CliampClient(), SpotifyClient.from_config())


def _default_playback_orchestrator():
    from cliamp_playback import CliampPlaybackAgent
    from orchestrator import Orchestrator

    return Orchestrator(CliampPlaybackAgent())


def _default_status_client():
    from cliamp_client import CliampClient

    return CliampClient()


def _item_label(item):
    if isinstance(item, str):
        return item
    if not isinstance(item, dict):
        return str(item)
    track = item.get("track")
    track_name = track.get("name") if isinstance(track, dict) else None
    return str(
        item.get("name")
        or item.get("title")
        or track_name
        or item.get("path")
        or item
    )


def _write_json(stream, payload):
    stream.write(json.dumps(payload, ensure_ascii=False) + "\n")


def _write_sections(stream, sections):
    for index, (key, label) in enumerate(SECTION_LABELS):
        if index:
            stream.write("\n")
        stream.write(f"{label}\n")
        items = sections.get(key, [])
        if not items:
            stream.write("  (none)\n")
        else:
            for item in items:
                source = item.get("source") if isinstance(item, dict) else None
                suffix = f" [{source}]" if source else ""
                stream.write(f"  {_item_label(item)}{suffix}\n")
    if sections.get("partial"):
        failed = ", ".join(sections.get("failed_sources", [])) or "unknown"
        stream.write(f"\nPartial results (unavailable: {failed})\n")


def _error_payload(exc):
    payload = getattr(exc, "payload", None)
    if isinstance(payload, dict):
        return payload
    if isinstance(exc, PermissionError):
        return {"status": "REJECTED", "reason": "OUT_OF_SCOPE"}
    return {"status": "TOOL_ERROR", "retry_allowed": False, "reason": str(exc)}


def main(argv=None, *, unified_client=None, playback_orchestrator=None,
         status_client=None, spotify_login=None, stdout=None, stderr=None):
    """Run the CLI and return its process exit code."""
    args = _parser().parse_args(argv)
    stdout = stdout or sys.stdout
    stderr = stderr or sys.stderr
    try:
        if args.command == "library":
            client = unified_client or _default_unified_client()
            result = client.get_library_sections()
            _write_json(stdout, result) if args.as_json else _write_sections(stdout, result)
        elif args.command == "recent":
            if args.limit < 1:
                raise ValueError("--limit must be at least 1")
            client = unified_client or _default_unified_client()
            result = client.get_recently_played(limit=args.limit)
            if args.as_json:
                _write_json(stdout, result)
            else:
                items = result.get("items", []) if isinstance(result, dict) else result
                for item in items:
                    stdout.write(f"{_item_label(item)}\n")
        elif args.command == "play":
            orchestrator = playback_orchestrator or _default_playback_orchestrator()
            telemetry = orchestrator.run(" ".join(args.instruction))
            response = telemetry.get("response", telemetry)
            if args.as_json:
                _write_json(stdout, response)
            elif response.get("status") == "SUCCESS":
                stdout.write("SUCCESS\n")
            else:
                _write_json(stderr, response)
                return 1
        elif args.command == "status":
            client = status_client or _default_status_client()
            result = client.get_status()
            if args.as_json:
                _write_json(stdout, result)
            else:
                stdout.write(f"{result.get('state', 'unknown')}\n")
                track = result.get("track")
                if track:
                    stdout.write(f"{_item_label(track)}\n")
        else:
            if spotify_login is None:
                from spotify_auth import login as spotify_login
            path = spotify_login(client_id=args.client_id)
            stdout.write(f"Spotify authorization saved to {path}\n")
    except Exception as exc:
        _write_json(stderr, _error_payload(exc))
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
