# Architecture

## The idea

Recording and editing are separate. While recording, only raw data is written:
the video without the cursor and a separate cursor log. Everything pretty
happens later, offline, and can be re-rendered with different settings without
re-recording.

```
Ctrl+Print ─┬─ wf-recorder-nc --no-cursor -D ─→ raw/X.mp4      (+ X.wf.log: time of the 1st frame)
            └─ curlog ──────────────────────→ raw/X.cursor.jsonl (positions + clicks)
stop ──→ smoothcast ──→ X.mp4 ──→ clipboard ──→ Video Trimmer
```

## Parts

Each part does one thing and lives in its own repository:

| Part | Language | What it does |
|---|---|---|
| [curlog](https://github.com/tihonove/curlog) | C, libwayland | cursor position via an ext-image-copy-capture cursor session, clicks from `/dev/input`; JSONL, `CLOCK_REALTIME` µs. The package installs a udev rule that opens mice/touchpads/trackpoints to the active session, but not keyboards |
| smoothcast (here) | Go, static binary | cursor spring and auto-zoom (a port of Capptivo, MIT — `LICENSE.capptivo`), background/shadow/rounding; doesn't touch frames — computes trajectories, draws PNGs and feeds ffmpeg per-frame commands via `sendcmd` |
| [wf-recorder](https://github.com/tihonove/wf-recorder) | C++ | fork of v0.6.0: branch `no-cursor` — the `--no-cursor` flag (for upstream), branch `nc` — packaging; installed as `wf-recorder-nc` |

The glue lives in dotfiles (`~/.dotfiles/modules/sway`): `screenrec` (Ctrl+Print —
region, Ctrl+Shift+Print — window, Ctrl+Win+Print — whole screen), `pickwin`, `shot`.

## Inside smoothcast

| File | What |
|---|---|
| `main.go` | flags, ffprobe, building the ffmpeg filter graph and the `sendcmd` script |
| `track.go` | reads the curlog log into a track: seconds from the first frame, coordinates as region fractions 0–1 |
| `cursor.go` | smooth cursor — an analytically stepped spring, snapping to the click point, reset after a gap (port of Capptivo `cursor.rs`) |
| `zoom.go` | auto-zoom — click series into zoom windows, follow-cursor camera (port of Capptivo `zoomSuggestionUtils.ts` and `zoom.rs`) |
| `assets.go` | static images computed once: background with shadow and a rounded window, the cursor from the xcursor theme |

Frames never pass through Go: ffmpeg overlays the static images on every frame,
and the per-frame cursor and camera positions are fed to it via `sendcmd`.

## Syncing video and cursor

The cursor log is in `CLOCK_REALTIME` µs; the video has to be pinned to the same
clock. No patch is needed for that: wf-recorder prints `pts_time: 0` to stderr
on the first frame, and screenrec stamps the lines with `$EPOCHREALTIME`; that
value goes to smoothcast as `-t0`. Without `-t0` the fallback is curlog's stop
minus the video duration; the two differ by ~26 ms.
