# Quickshell Now-Playing Widget (Omarchy)

A notification-sized "now playing" card for [Quickshell](https://quickshell.org), tailored for [Omarchy](https://omarchy.org). Lives in the repo at [`contrib/quickshell/`](../contrib/quickshell).

It pulls live spectrum data from a running cliamp over the IPC socket, draws a Winamp 2-style segmented analyzer, and reads colors directly from the active Omarchy theme so it restyles in lock-step when you swap themes.

> This widget is built for an Omarchy setup. It uses Omarchy's `~/.config/omarchy/current/theme/colors.toml` as the source of truth for its palette. On a non-Omarchy system the panel still works but falls back to the default kanagawa-dragon-ish colors baked into `NowPlaying.qml`.

## What you get

- 300 x 72 card centered along the bottom of every screen
- Full-width Winamp 2-style LED spectrum analyzer with falling peak caps
- Track title + artist, click-to-seek progress bar, time readout
- Pristine vector transport icons (prev / play-pause / next), no font dependency
- Theme follows the active Omarchy theme (background, foreground, accent, color1..color8, selection_background)
- Card border tracks the muted slot of the Omarchy palette (`selection_background`, falling back to `color8`)
- Click the card and press `Esc` or `Q` to dismiss

## Quick start

Make sure cliamp is running first, then either run the widget directly:

```sh
qs -p contrib/quickshell/shell.qml
```

Or install it as a named Quickshell config:

```sh
mkdir -p ~/.config/quickshell
ln -s "$PWD/contrib/quickshell" ~/.config/quickshell/cliamp
qs -c cliamp
```

## Requirements

- Quickshell 0.2+
- A running cliamp (Linux only; the widget needs cliamp's MPRIS service and IPC socket)
- Omarchy, for the theme integration. Without Omarchy the palette stays on the built-in defaults.

