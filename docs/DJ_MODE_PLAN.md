# DJ Mode — Implementation Plan

Status: Foundation implemented; UI/IPC delivery in progress
Scope: cliamp-clify (Go player + TUI) + packages/clify (Python ADD framework)
Target: v1 includes transitions, dual-deck turntable control, multi-track mixing,
Auto-DJ, BPM detect + Sync, hot cues, beat loops, IPC + Python DjAgent.

## Design Decisions

- DJ core lives inside the `player` package (not a subpackage): `buildPipelineAt`,
  decoders, and resamplers are unexported — decks must reuse them.
  New files: `player/dj_mixer.go`, `player/dj_deck.go`, `player/fader.go`,
  `player/transition.go`, `player/bpm.go`.
- New `player.DJEngine` interface instead of extending `Engine` — avoids breaking
  existing fake engines in tests; `Model` gets an optional `dj DJEngine` field,
  nil when DJ mode is off.
- Topology:

  ```
  Deck A pipeline ─► faderA ─┐
                             ├──► beep.Mixer ─► [global chain: EQ→Tap→Vol→Ctrl] ─► speaker
  Deck B pipeline ─► faderB ─┘
  ```

- Per-deck FFT taps feed dual VU meters; global chain untouched during normal playback.

## Phases

### Phase 1 — Dual-deck engine core (`player/`) 🚧 foundation complete
- `fader.go`: atomic-gain `faderStreamer` with sample/block ramps
  (template: volume.go cached-gain pattern). Equal-power crossfade curves.
- `dj_deck.go`: deck = pipeline + position + pause/cue state; reuses
  `buildPipelineAt`; graceful handling of Spotify streamers and non-seekable yt-dlp sources.
- `dj_mixer.go`: `djMixer` (beep.Streamer) used only when DJ mode active;
  single-deck gapless path remains default otherwise.
- `transition.go`: crossfade state machine — manual fader, auto-fade on
  end-of-track, cut/fade/brake styles.
- Tests: sine/fake streamers, table-driven ramp assertions, fade-curve continuity (no clicks).

Implemented foundation: `faderStreamer`, equal-power gains, `djMixer`,
transition state, `DJDeck`, `DJEngine`, `DJController`, and player/model
integration seams. The current controller is deliberately pipeline-independent;
deck loading and speaker-graph activation remain Phase 1 follow-up work.

### Phase 2 — BPM detect + Sync (`player/bpm.go`) 🚧 estimator complete
- Onset-energy autocorrelation over PCM sampled during deck load
  (background goroutine), confidence score.
- Sync button adjusts idle-deck WSOLA speed to match lead deck BPM.
- Low-confidence tracks disable sync.

Implemented: deterministic PCM onset-energy autocorrelation with BPM and
confidence output, plus confidence-gated pitch-ratio sync in `DJController`.
Background decoder sampling and WSOLA are still pending.

### Phase 3 — TUI DJ screen (`ui/model/`) 🚧 first screen complete
- Established screen recipe: `screenDJMode` constant, `djState` in state.go,
  `handleDJKey`, full-screen takeover case in View(), `commandModeDJ` rows in
  command_registry.go, Ctrl+K help integration.
- Layout: decks A/B side-by-side (transport, pitch slider ±16%, key-lock toggle,
  VU meter), central crossfader bar, waveform overview strips from PCM peaks at load.
- Keys (proposed): `1/2` focus deck A/B, `[ ]` pitch-bend nudge, `-/+` pitch,
  `\` crossfader slide, `c` crossfade, `s` sync, `b` brake, `Tab` cycle focus.
- Auto-DJ: preloads next playlist track into idle deck; auto-crossfades when lead
  deck reaches (duration − fade length).

Implemented first screen: `D` opens the DJ view, the bottom status line advertises
`[D] DJ`, and the screen supports deck focus, crossfader movement/centering,
pitch nudge, and confidence-gated sync. Auto-DJ, live deck audio routing, and
the remaining transport controls are still pending.

### Phase 4 — Performance tools
- Hot cues: 8 slots/deck, session-persistent v1, jump-on-trigger.
- Beat loops: A/B loop points + bar-length loops (4/8/16 beats via BPM); loop roll.
- FX per deck: filter knob (biquad LPF/HPF sweep), EQ kill (3-band biquads),
  echo send (feedback delay buffer).
- Sampler pads: 8 one-shot triggers mixed into djMixer.

### Phase 5 — Mixtape recording
- Post-mix tap → WAV writer → `~/.config/cliamp/recordings/`;
  start/stop via TUI + IPC; timestamped filenames.

### Phase 6 — Integration (Go ↔ Python) 🚧 Python seam complete
- IPC: `dj.status/load/play/pause/crossfade/pitch/nudge/sync/cue/loop/fx/sample/
  record/autodj` in ipc/server.go dispatch + daemon parity + reply-channel pattern;
  `cliamp dj ...` CLI subcommands.
- Python: extend `CliampController` (packages/clify/cliamp_controller.py) with dj
  methods; new `DjAgent` in cliamp_agents.py + `agent_manifest.dj.json`
  (scopes: `dj.read`, `dj.control`; prohibited: `library.write`);
  NL routing ("blend into the next song" → crossfade); SLA telemetry per root
  AGENTS.md spec; pytest suite mirroring test_cliamp_playback_agent.py patterns.

Implemented: `DjAgent`, `agent_manifest.dj.json`, controller DJ methods, and
scope-gated natural-language routing. Go IPC dispatch, daemon handlers, and
the public `cliamp dj ...` command surface remain pending.

### Phase 7 — Polish
- `docs/dj.md`, keybindings.md update, site/index.html sync (repo-mandated),
  CHANGELOG entry, ROADMAP.md backlog reconciliation.

## Known Constraints (documented, not blocking)

- Spotify: go-librespot may reject two concurrent streams per session — local files
  are first-class for DJing; Spotify decks work sequentially. Surfaced as a status
  warning on the DJ screen.
- yt-dlp tracks: pipe-based, no seek → cue/seek limited; flagged in deck UI.
- No headphone cue channel (single output device).

## Verification (after each phase)

```sh
cd cliamp-clify && make check && go test -race ./...
cd ../packages/clify && python -m pytest tests/   # Phase 6+
```

Layout/rendering changes must pass frame-budget tests (`assertViewFits`).
