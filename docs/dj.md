# DJ mode

DJ mode is being delivered as an optional dual-deck layer for `cliamp-clify`.
Normal single-deck playback remains unchanged.

## Current status

The engine foundation is available in the Go `player` package:

- `DJEngine` is separate from the existing `Engine` interface, preserving
  compatibility with existing player fakes.
- Two deck state objects and a mixer/fader seam are available.
- Faders use sample ramps and equal-power crossfade curves.
- `DetectBPM` estimates tempo and reports confidence for sync decisions.
- Sync refuses low-confidence BPM estimates.
- The model accepts an optional DJ engine with `SetDJEngine`.

The TUI DJ screen and controls are now enabled with `D` when the player
provides a DJ engine. Live speaker-graph deck mixing, Go IPC handlers, and the
`cliamp dj ...` command group are not yet enabled. See
[`DJ_MODE_PLAN.md`](DJ_MODE_PLAN.md) for phase status.

## Python agent

The ADD framework includes `DjAgent`, scoped to `dj.read` and `dj.control`.
It routes concise requests to the injected `CliampController` and rejects
library writes and billing operations before tool execution.

```python
from cliamp_agents import DjAgent

agent = DjAgent()
agent.process_instruction("blend into the next song")
agent.process_instruction("sync deck B")
```

The manifest is [`packages/clify/agent_manifest.dj.json`](../packages/clify/agent_manifest.dj.json).
The controller methods are currently a subprocess seam for the future Go
`dj.*` command surface; run them against a daemon only after that surface is
available.

## Build and test

```sh
cd cliamp-clify
GOCACHE=/tmp/clify-go-cache go test ./player
go build -trimpath -ldflags="-s -w -X main.version=dj-mode-dev" \
  -o bin/cliamp-clify .

cd ../packages/clify
python -m pytest -q
```
