# Themes

cliamp ships with 20 built-in color themes and supports custom themes via simple TOML files.

Press `t` during playback to open the theme picker. Navigate with `↑`/`↓`, preview live as you move, confirm with `Enter`, or cancel with `Esc`.

Your selection is saved automatically and restored on next launch.

## Built-in themes

ayu-mirage-dark, catppuccin, catppuccin-latte, dracula, ember, ethereal, everforest, flexoki-light, gruvbox, hackerman, kanagawa, matte-black, miasma, neon-blade-runner, nord, osaka-jade, ristretto, rose-pine, tokyo-night, vantablack

## Creating a custom theme

Create a `.toml` file in `~/.config/cliamp/themes/`:

```
mkdir -p ~/.config/cliamp/themes
```

Each file needs all 6 colors as `#RRGGBB` hex values. Incomplete or malformed
custom themes are ignored, so they cannot silently make focus, warning, error,
or disabled states unreadable. The filename (minus `.toml`) becomes the theme name.

### Example: `~/.config/cliamp/themes/solarized.toml`

```toml
accent = "#268bd2"
bright_fg = "#eee8d5"
fg = "#839496"
green = "#859900"
yellow = "#b58900"
red = "#dc322f"
```

That's it. Press `t` and your theme appears in the list immediately.

### Color reference

| Key         | What it colors                                    |
|-------------|---------------------------------------------------|
| `accent`    | Title, track name, seek bar, selected items       |
| `bright_fg` | Primary text, time display, help key pill text     |
| `fg`        | Muted/secondary text, help bar, inactive elements, help key pill background |
| `green`     | Playing indicator, volume bar, spectrum low        |
| `yellow`    | Spectrum middle                                   |
| `red`       | Spectrum top, error messages                      |

All values are six-digit hex strings (for example, `"#ff5733"`).

Important UI states also use stable text markers such as `>`, `Q`, `★`, and `!`,
so selection, queued, bookmarked, and unavailable tracks remain distinguishable
in monochrome terminals.

## Overriding a built-in theme

If your custom file has the same name as a built-in theme, yours takes priority. For example, creating `~/.config/cliamp/themes/catppuccin.toml` replaces the built-in catppuccin.

## Omarchy desktop theme (live sync)

On [Omarchy](https://github.com/omarchy/omarchy) systems, cliamp can follow your
active desktop palette automatically.

When `theme` is empty or omitted in `~/.config/cliamp/config.toml`, cliamp reads
`~/.config/omarchy/current/theme/colors.toml` on launch and maps Omarchy keys
onto cliamp's six-color palette (`accent`, `bright_fg`, `fg`, `green`, `yellow`,
`red`). Modern Omarchy keys (`green`, `yellow`, `red`) and legacy slots
(`color2`, `color3`, `color1`) are both supported.

While Omarchy sync is active, cliamp polls that file every ~2 seconds. When you
run `omarchy theme set <name>`, UI accents and **all spectrum visualizers** pick
up the new colors without restarting cliamp.

The live palette also appears in the theme picker (`t`) as **omarchy**. Selecting
it enables the same hot-reload behaviour. Choosing **default** or any other
built-in/custom theme disables Omarchy sync for that session.

## Setting a default theme

Add a `theme` line to `~/.config/cliamp/config.toml`:

```toml
theme = "catppuccin"
```

Use the filename without `.toml`. Leave empty or omit to use terminal default
colors, or — on Omarchy — to auto-sync with your desktop theme (see above).

To pin Omarchy explicitly:

```toml
theme = "omarchy"
```
