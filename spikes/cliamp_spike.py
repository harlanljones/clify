"""Phase 0 spike: round-trip cliamp playlist show / history via subprocess.

Validates the transport decision (subprocess + --json, stateless) against a
real cliamp install. Not a unit test — run manually:

    python spikes/cliamp_spike.py
"""

import json
import subprocess
import sys

CLIAMP = "cliamp"
TIMEOUT = 2.0
SPIKE_PLAYLIST = "clify_spike_tmp"


def run(argv):
    """Run cliamp, returning (exit_code, parsed_json_or_None, raw_stdout)."""
    proc = subprocess.run(argv, capture_output=True, text=True, timeout=TIMEOUT)
    payload = None
    if proc.stdout.strip():
        try:
            payload = json.loads(proc.stdout)
        except json.JSONDecodeError:
            pass
    return proc.returncode, payload, proc.stdout.strip(), proc.stderr.strip()


def main():
    # --- history round-trip -------------------------------------------------
    rc, history, raw, err = run([CLIAMP, "history", "--json"])
    print(f"history --json: exit={rc} type={type(history).__name__} "
          f"entries={len(history) if isinstance(history, list) else 'n/a'}")
    if rc != 0 or not isinstance(history, list):
        print(f"FAIL: history round-trip (stderr={err!r})")
        return 1

    # --- playlist show round-trip -------------------------------------------
    # Create a throwaway playlist so `show` has something to return.
    rc, _, _, err = run([CLIAMP, "playlist", "create", SPIKE_PLAYLIST])
    print(f"playlist create {SPIKE_PLAYLIST!r}: exit={rc} {err}")
    if rc != 0 and "exists" not in err:
        print("FAIL: could not create spike playlist")
        return 1

    rc, show, raw, err = run([CLIAMP, "playlist", "show", SPIKE_PLAYLIST, "--json"])
    print(f"playlist show --json: exit={rc}")
    print(f"  payload: {json.dumps(show) if show is not None else raw!r}")

    # Negative case: missing playlist must exit non-zero.
    rc, _, _, err = run([CLIAMP, "playlist", "show", "clify_no_such_playlist", "--json"])
    print(f"playlist show (missing): exit={rc} stderr={err!r}")
    missing_ok = rc != 0

    # Cleanup.
    rc, _, _, err = run([CLIAMP, "playlist", "delete", SPIKE_PLAYLIST])
    print(f"playlist delete cleanup: exit={rc} {err}")

    ok = isinstance(show, (dict, list)) and missing_ok
    print("SPIKE RESULT:", "PASS" if ok else "FAIL")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
