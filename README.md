# smoothcast

Screen recording "like Screen Studio" on sway, built from small tools: a smooth
large cursor, auto-zoom on clicks, a background with a shadow and rounded
corners. No Electron, no portal, no Python or Rust at runtime: compiled
binaries plus ffmpeg.

## Idea

Recording and editing are separate. While recording, only raw data is written:
the video without the cursor and a separate cursor log. Everything pretty
happens later, offline, and can be re-rendered with different settings without
re-recording.

```
Ctrl+Print ─┬─ wf-recorder-nc --no-cursor -D ─→ raw/X.mp4      (+ X.wf.log: time of the 1st frame)
            └─ curlog ──────────────────────→ raw/X.cursor.jsonl (positions + clicks)
stop ──→ smoothcast ──→ X.mp4 ──→ clipboard ──→ Video Trimmer
```

Each part does one thing and lives in its own repository:

| Part | Language | What it does |
|---|---|---|
| [curlog](https://github.com/tihonove/curlog) | C, libwayland | cursor position via an ext-image-copy-capture cursor session, clicks from `/dev/input`; JSONL, `CLOCK_REALTIME` µs. The package installs a udev rule that opens mice/touchpads/trackpoints to the active session, but not keyboards |
| smoothcast (here) | Go, static binary | cursor spring and auto-zoom (a port of Capptivo, MIT — `LICENSE.capptivo`), background/shadow/rounding; doesn't touch frames — computes trajectories, draws PNGs and feeds ffmpeg per-frame commands via `sendcmd` |
| [wf-recorder](https://github.com/tihonove/wf-recorder) | C++ | fork of v0.6.0: branch `no-cursor` — the `--no-cursor` flag (for upstream), branch `nc` — packaging; installed as `wf-recorder-nc` |

The glue lives in dotfiles (`~/.dotfiles/modules/sway`): `screenrec` (Ctrl+Print —
region, Ctrl+Shift+Print — window, Ctrl+Win+Print — whole screen), `pickwin`, `shot`.

## Install

Packages for Ubuntu 24.04 and 26.04, amd64 and arm64, come from the apt
repository [tihonove/apt](https://github.com/tihonove/apt): `smoothcast`,
`curlog`, `wf-recorder-nc`.

```sh
curl -fsSL https://tihonove.github.io/apt/install.sh | sudo sh
```

Releasing a version: tag `vX.Y.Z` in the tool's repository. CI builds the
`.deb` files and attaches them to a GitHub Release; the apt repository picks
them up by itself.

From source: `make install` (into `~/.local/bin`, needs Go).

## Learned along the way

- Capturing with the cursor baked in (wlr-screencopy with `overlay_cursor=1`:
  wf-recorder, `grim -c`) kills sway 1.11's cursor session: `leave` right away,
  no more positions. Hence the fork: with no cursor in the frame curlog gets
  every movement.
- Without the cursor in the frame its movement produces no damage, so
  wf-recorder gets no frames on a static screen. Hence `-D`; the cost is about
  one extra core at 3440×1440.
- curlog has to open `get_capture_session`, otherwise the cursor source sends
  `leave` right away, even with no recording.
- Sync without a patch: wf-recorder prints `pts_time: 0` to stderr on the first
  frame, and screenrec stamps the lines with `$EPOCHREALTIME`. The fallback is
  curlog's stop minus the duration; the two differ by ~26 ms.
- `swaymsg seat seat0 cursor set X Y` (output-relative coordinates) works for
  testing curlog without a real mouse.
- Touchpad taps are invisible in evdev — only a button press counts as a click.
- xdg-desktop-portal-wlr 0.8.1 on sway 1.11 delivers no frames, fixed in 0.8.4;
  Capptivo was tried and rolled back — not the Linux way.

## Upstreaming `--no-cursor` to wf-recorder

State as of 2026-10-21 (`ammen99/wf-recorder`, 1.3k stars, release 0.6.0 —
October 2025):

- **The maintainer is active.** ammen99 merges himself; small PRs land in 1–14
  days (#344, #339, #337, #334, #318, #317, #315, etc.). Issues usually get
  closed within days. There are long-lived ones too (#256, #236 from
  2023–2024) — mostly contentious or unfinished.
- **Nobody has asked for the flag.** Searching issues/PRs (cursor, pointer, hide
  mouse, overlay_cursor, no-cursor) finds no requests. A cheap change with a
  clear motivation and no effect on default behavior is a good candidate.
- **The main risk is PR #347 (soreau, "Port to ext-copy-capture").** It drops
  wlr-screencopy entirely, and the cursor is hardcoded there too:
  `create_session(…, OPTIONS_PAINT_CURSORS)`. soreau is the second regular
  contributor (from the Wayfire team); the PR has been open since June, is
  referenced from #273 and waits for the wlroots 0.20 release. wlroots plans to
  remove wlr-screencopy altogether (wlroots MR 5149), so the future is
  ext-copy-capture.

**Plan:**

1. No need to scout with an issue first — go straight to a small PR against
   `master`: `--no-cursor` (10 lines, man page, `--help`). The description gives
   the "cursor is drawn separately, later" use case and points out that `grim`
   (without `-c`) and wl-screenrec (`--no-cursor`) already do this.
2. In parallel — a comment/PR on soreau's `ext-copy-capture` branch: the same
   option as `options = no_cursor ? 0 : PAINT_CURSORS`. That way the flag
   survives the port whichever lands first. #347 doesn't touch argument parsing
   or `--help` (8 of the patch's 10 lines) — only the line where the flag is
   used changes.

   **Unverified:** whether the cursor session survives next to an
   ext-copy-capture capture without the cursor. To check: build soreau's branch
   with that line (`wf-recorder-ecc/`), run curlog + `swaymsg seat seat0 cursor
   set` as with grim. If it breaks there too, that's an argument for a wlroots
   bug (item 3).
3. Separately, report to wlroots/sway that wlr-screencopy's `overlay_cursor=1`
   breaks the ext-image-copy-capture cursor session — a bug, not a feature; the
   minimal reproducer is curlog + `grim -c` in a loop.
4. Once everything is merged, `wf-recorder-nc` is no longer needed and
   screenrec switches to the system wf-recorder as soon as Ubuntu's package
   catches up.

## Next

In order of importance:

1. **Patch and send upstream** (see above).
2. **Zoom on screen changes.** The H.264 skip-block map from
   `ffmpeg -debug mb_type -threads 1` (checked — works): after a click, look at
   where the screen changed and fit the zoom box so it holds both the click and
   the change; if almost the whole screen changed, don't zoom. The same signal
   catches touchpad taps: the cursor stood still and the screen changed next to
   it — a click.
3. **Cursor**: auto-hide when idle (Capptivo has it, not ported), cursor shape
   (hand over links, I-beam) — the cursor session provides the image; a
   "squeeze" on click.
4. **Frame quality**: crop in yuv444 so a slow camera doesn't jitter by 1–2 px;
   pick a default `-cursor-size`.
5. **Convenience**: defaults in a config file rather than flags; re-rendering
   right from Video Trimmer or via a separate `screenrec --rebuild`.
6. **A window rather than a screen area**: needs ext-copy-capture toplevel
   capture — comes with PR #347.
