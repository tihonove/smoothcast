# smoothcast

Turns a raw screen recording into a polished demo video, "like Screen Studio":
a smooth large cursor, auto-zoom on clicks, a background with a shadow and
rounded corners.

smoothcast doesn't record anything. It takes two files — a video **without**
the cursor and a log of cursor positions and clicks — draws the cursor back in,
smooths its movement and moves the camera toward the clicks. A single static
binary plus ffmpeg; the inputs are never modified, so you can re-render with
different settings as many times as you like.

## Install

Ubuntu 24.04 / 26.04 (amd64, arm64) — package `smoothcast` from the apt
repository [tihonove/apt](https://github.com/tihonove/apt):

```sh
curl -fsSL https://tihonove.github.io/apt/install.sh | sudo sh
sudo apt install smoothcast
```

From source: `make install` (into `~/.local/bin`, needs Go). At runtime it needs
`ffmpeg` and `ffprobe`.

## Usage

```sh
smoothcast demo.mp4                 # reads demo.cursor.jsonl → writes demo-smooth.mp4
```

Not happy with the result? Change the flags and run it again:

```sh
smoothcast -zoom 2 -smoothness 0.7 -bg wallpaper.jpg -o final.mp4 demo.mp4
```

| Flag | Default | What it does |
|---|---|---|
| `-o` | `<recording>-smooth.mp4` | output file |
| `-cursor` | `<recording>.cursor.jsonl` | cursor log |
| `-bg` | gradient | background image |
| `-width` | `1920` | long side of the output; `0` — same as the source |
| `-padding` | `120` | padding around the recording (px per 1920) |
| `-radius` | `18` | corner radius (px per 1920) |
| `-shadow` | `75` | shadow strength, 0–200 |
| `-smoothness` | `0.45` | cursor smoothness, 0–1 |
| `-cursor-size` | `36` | cursor height in the output, px (before zoom) |
| `-zoom` | `1.5` | auto-zoom magnification; `1` disables zoom |
| `-zoom-hold` | `3` | clicks closer than this many seconds share one zoom |
| `-crf` | `20` | x264 quality (lower is better and bigger) |
| `-t0` | from the log | time of the first video frame, µs — see below |

## Input

**Video** — anything ffmpeg can read, with the cursor *not* drawn into the frames.

**Cursor log** — JSONL in the [curlog](https://github.com/tihonove/curlog)
format. Time is in microseconds (any clock, as long as `-t0` uses the same one),
coordinates are pixels of the recorded area:

```jsonl
{"v":1,"region":[0,0,1920,1080]}
{"t":1761000000000000,"x":812,"y":440}
{"t":1761000000016000,"x":815,"y":442}
{"t":1761000000500000,"ev":"down","btn":"left"}
{"t":1761000000580000,"ev":"up","btn":"left"}
{"t":1761000012000000,"ev":"stop"}
```

- The first line is a header; only `region` (its width and height) is used.
- Lines with `x`/`y` are cursor positions; `"ev":"down","btn":"left"` is a
  click — only left-button presses drive the zoom.
- Video and log are aligned by `-t0`, the log time of the first video frame.
  Without it smoothcast assumes the recording and the log ended together:
  first frame = `stop` event − video duration (accurate to a few tens of ms).

## Recording the inputs

Any tool that produces the two files will do. The author's setup on sway uses
[curlog](https://github.com/tihonove/curlog) for the log and
[wf-recorder-nc](https://github.com/tihonove/wf-recorder) (wf-recorder with a
`--no-cursor` flag) for the video, both in the same apt repository:

```sh
geom=$(slurp)                       # pick a region (keep width/height even for x264)
curlog -g "$geom" -f demo.cursor.jsonl &
wf-recorder-nc --no-cursor -D -g "$geom" -r 60 -f demo.mp4
# Ctrl+C to stop, then:
kill -INT %1
```

Compositor requirements and caveats are those tools' business — see their
READMEs. A one-key wrapper (`Ctrl+Print` to start and stop, result in the
clipboard) is `screenrec` in the author's
[dotfiles](https://github.com/tihonove/dotfiles) (`modules/sway`).

## Limitations

- The cursor is always the arrow from your xcursor theme; no hand / I-beam yet.
- Zoom reacts to clicks only, not to what changes on screen.

## More

- [Architecture](docs/architecture.md) — the idea, the pipeline and how the parts fit together
- [Lessons learned](docs/lessons-learned.md) — sway / wlroots pitfalls found while building the recording side
- [Upstreaming `--no-cursor`](docs/upstream.md) — plan for getting rid of the wf-recorder fork
- [Roadmap](docs/roadmap.md)
- [Development](docs/development.md) — building and releasing

## License

See [`LICENSE`](LICENSE). Cursor and zoom logic is a port of Capptivo
(MIT, © 2026 idboussadel) — see [`LICENSE.capptivo`](LICENSE.capptivo).
