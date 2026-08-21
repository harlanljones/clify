import io
import json
from unittest.mock import Mock

from clify_cli import main


SECTIONS = {
    "recently_played": [{"title": "Newest", "source": "spotify"}],
    "library": [{"name": "Liked Songs", "source": "spotify"}],
    "your_playlists": ["Local Mix"],
    "partial": False,
    "failed_sources": [],
}


def run_cli(argv, **dependencies):
    stdout, stderr = io.StringIO(), io.StringIO()
    code = main(argv, stdout=stdout, stderr=stderr, **dependencies)
    return code, stdout.getvalue(), stderr.getvalue()


def test_library_text_has_fixed_section_order():
    client = Mock()
    client.get_library_sections.return_value = SECTIONS
    code, output, error = run_cli(["library"], unified_client=client)
    assert code == 0 and not error
    assert output.index("Recently Played") < output.index("Library")
    assert output.index("Library") < output.index("Your Playlists")
    assert "Newest [spotify]" in output


def test_library_json_preserves_insertion_order():
    client = Mock()
    client.get_library_sections.return_value = SECTIONS
    code, output, _ = run_cli(["library", "--json"], unified_client=client)
    assert code == 0
    assert list(json.loads(output))[:3] == [
        "recently_played", "library", "your_playlists"
    ]


def test_recent_limit_is_forwarded():
    client = Mock()
    client.get_recently_played.return_value = [{"title": "One"}]
    code, output, _ = run_cli(["recent", "--limit", "1"], unified_client=client)
    assert code == 0 and output == "One\n"
    client.get_recently_played.assert_called_once_with(limit=1)


def test_play_routes_instruction_through_orchestrator():
    orchestrator = Mock()
    orchestrator.run.return_value = {
        "response": {"status": "SUCCESS", "data": [{"state": "playing"}]}
    }
    code, output, _ = run_cli(
        ["play", "toggle", "playback"], playback_orchestrator=orchestrator
    )
    assert code == 0 and output == "SUCCESS\n"
    orchestrator.run.assert_called_once_with("toggle playback")


def test_status_passthrough():
    client = Mock()
    client.get_status.return_value = {"state": "paused", "track": {"title": "Song"}}
    code, output, _ = run_cli(["status"], status_client=client)
    assert code == 0 and output == "paused\nSong\n"


def test_tool_error_is_structured_and_nonzero():
    client = Mock()
    client.get_library_sections.side_effect = RuntimeError("provider unavailable")
    code, _, error = run_cli(["library"], unified_client=client)
    assert code == 1
    assert json.loads(error) == {
        "status": "TOOL_ERROR",
        "retry_allowed": False,
        "reason": "provider unavailable",
    }


def test_rejected_playback_is_nonzero():
    orchestrator = Mock()
    orchestrator.run.return_value = {
        "response": {"status": "REJECTED", "reason": "OUT_OF_SCOPE"}
    }
    code, _, error = run_cli(["play", "delete history"], playback_orchestrator=orchestrator)
    assert code == 1
    assert json.loads(error)["status"] == "REJECTED"


def test_spotify_login_routes_to_auth_flow(tmp_path):
    target = tmp_path / "spotify.json"
    login = Mock(return_value=target)
    code, output, error = run_cli(["spotify", "login"], spotify_login=login)
    assert code == 0 and not error
    assert str(target) in output
    login.assert_called_once_with(client_id=None)


def test_spotify_login_accepts_public_client_id(tmp_path):
    login = Mock(return_value=tmp_path / "spotify.json")
    code, _, _ = run_cli(
        ["spotify", "login", "--client-id", "public-id"], spotify_login=login
    )
    assert code == 0
    login.assert_called_once_with(client_id="public-id")
