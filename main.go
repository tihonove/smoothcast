// smoothcast re-renders a screen recording "like Screen Studio": a smooth cursor
// over the recording, auto-zoom on clicks, a background with a shadow and
// rounded corners.
//
//	smoothcast [flags] recording.mp4
//
// Expects the curlog log next to the recording (recording.cursor.jsonl). Frames
// never pass through Go: this computes trajectories and images, ffmpeg assembles
// the video — per-frame cursor and camera positions are fed to it via sendcmd.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	var (
		logPath    = flag.String("cursor", "", "curlog log (default <recording>.cursor.jsonl)")
		outPath    = flag.String("o", "", "output file (default <recording>-smooth.mp4)")
		t0         = flag.Int64("t0", 0, "first frame time, µs CLOCK_REALTIME (otherwise: curlog stop − duration)")
		bgPath     = flag.String("bg", "", "background image (default: gradient)")
		width      = flag.Int("width", 1920, "long side of the output, 0 — same as the scene")
		padding    = flag.Float64("padding", 120, "padding around the recording, px per 1920 of the long side")
		radius     = flag.Float64("radius", 18, "corner radius, px per 1920")
		shadow     = flag.Float64("shadow", 75, "shadow strength, 0–200")
		smoothness = flag.Float64("smoothness", 0.45, "cursor smoothness, 0–1")
		cursorSize = flag.Int("cursor-size", 36, "cursor height in the output video, px (before zoom)")
		zoom       = flag.Float64("zoom", 1.5, "auto-zoom magnification, 1 — no zoom")
		zoomHold   = flag.Float64("zoom-hold", 3, "zooms closer than this, s, merge into one — the camera stays in")
		crf        = flag.Int("crf", 20, "x264 quality")
	)
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: smoothcast [flags] recording.mp4")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	in := flag.Arg(0)
	stem := strings.TrimSuffix(in, filepath.Ext(in))
	if *logPath == "" {
		*logPath = stem + ".cursor.jsonl"
	}
	if *outPath == "" {
		*outPath = stem + "-smooth.mp4"
	}

	opt := options{
		bg: *bgPath, width: *width, padding: *padding, radius: *radius, shadow: *shadow,
		smoothness: *smoothness, cursorSize: *cursorSize, zoom: *zoom, zoomHold: *zoomHold, crf: *crf,
	}
	if err := run(in, *logPath, *outPath, *t0, opt); err != nil {
		fmt.Fprintln(os.Stderr, "smoothcast:", err)
		os.Exit(1)
	}
}

type options struct {
	bg                      string
	width                   int
	padding, radius, shadow float64
	smoothness              float64
	cursorSize              int
	zoom                    float64
	zoomHold                float64
	crf                     int
}

type videoInfo struct {
	w, h     int
	fps      float64
	duration float64
}

func probe(path string) (videoInfo, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate:format=duration", "-of", "json", path).Output()
	if err != nil {
		return videoInfo{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	var p struct {
		Streams []struct {
			Width, Height int
			RFrameRate    string `json:"r_frame_rate"`
		}
		Format struct{ Duration string }
	}
	if err := json.Unmarshal(out, &p); err != nil || len(p.Streams) == 0 {
		return videoInfo{}, fmt.Errorf("ffprobe %s: no video stream", path)
	}
	s := p.Streams[0]
	v := videoInfo{w: s.Width, h: s.Height}
	v.duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	if n, d, ok := strings.Cut(s.RFrameRate, "/"); ok {
		a, _ := strconv.ParseFloat(n, 64)
		b, _ := strconv.ParseFloat(d, 64)
		if b > 0 {
			v.fps = a / b
		}
	}
	if v.fps <= 0 || v.fps > 240 {
		v.fps = 60
	}
	return v, nil
}

func even(x float64) int { return int(math.Round(x/2)) * 2 }

func run(in, logPath, outPath string, t0 int64, opt options) error {
	v, err := probe(in)
	if err != nil {
		return err
	}
	tr, _, err := loadTrack(logPath)
	if err != nil {
		return err
	}
	if t0 == 0 {
		if tr.stop == 0 {
			return fmt.Errorf("%s: no stop event, pass -t0", logPath)
		}
		t0 = tr.stop - int64(v.duration*1e6)
	}
	if int(tr.w) != v.w || int(tr.h) != v.h {
		fmt.Fprintf(os.Stderr, "smoothcast: warning: log region %gx%g, video %dx%d\n", tr.w, tr.h, v.w, v.h)
		tr.w, tr.h = float64(v.w), float64(v.h)
	}
	tr.rebase(t0)

	// The scene in recording pixels: the recording 1:1, padding around, the camera on top.
	k := float64(max(v.w, v.h)) / 1920
	pad := even(opt.padding * k)
	sw, sh := v.w+2*pad, v.h+2*pad
	video := [4]float64{float64(pad), float64(pad), float64(v.w), float64(v.h)}
	ow, oh := sw, sh
	if opt.width > 0 {
		s := float64(opt.width) / float64(max(sw, sh))
		ow, oh = even(float64(sw)*s), even(float64(sh)*s)
	}

	frames := int(math.Round(v.duration * v.fps))
	cursor := smoothCursor(tr, frames, v.fps, opt.smoothness)
	var frags []zoomFragment
	if opt.zoom > 1 {
		frags = suggestZooms(tr.clicks, v.duration, opt.zoom, opt.zoomHold)
	}
	cam := newCamera(frags, tr)
	fmt.Fprintf(os.Stderr, "smoothcast: %dx%d %.1fs, %d moves, %d clicks, %d zooms\n",
		v.w, v.h, v.duration, len(tr.samples), len(tr.clicks), len(frags))

	tmp, err := os.MkdirTemp("", "smoothcast-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	bg := gradient(sw, sh)
	if opt.bg != "" {
		if bg, err = imageBackground(opt.bg, sw, sh, tmp); err != nil {
			return err
		}
	}
	framePNG := filepath.Join(tmp, "frame.png")
	if err := writePNG(framePNG, frameImage(bg, sw, sh, video, opt.radius*k, opt.shadow)); err != nil {
		return err
	}
	// Cursor size is in output pixels: the scene is later scaled to -width, and
	// the same size in recording pixels would give a huge arrow on a small region
	// and a tiny one on a large monitor.
	cursorPx := float64(opt.cursorSize) * float64(sw) / float64(ow)
	cimg, hot, err := xcursor(int(math.Round(cursorPx)))
	if err != nil {
		return err
	}
	cursorPNG := filepath.Join(tmp, "cursor.png")
	if err := writePNG(cursorPNG, cimg); err != nil {
		return err
	}
	cs := cursorPx / float64(cimg.Bounds().Dy())
	cw, ch := even(float64(cimg.Bounds().Dx())*cs), even(float64(cimg.Bounds().Dy())*cs)

	// Per-frame commands. Timed half a frame early so each command surely lands
	// before its frame; repeats are skipped.
	cmdPath := filepath.Join(tmp, "cmds.txt")
	var b strings.Builder
	var prev [6]int
	for i := 0; i < frames; i++ {
		t := float64(i) / v.fps
		p := cursor[i]
		cx, cy, cw2, ch2 := cam.rect(t, float64(sw), float64(sh), video)
		cur := [6]int{
			int(math.Round(video[0] + p.x*video[2] - float64(hot.X)*cs)),
			int(math.Round(video[1] + p.y*video[3] - float64(hot.Y)*cs)),
			int(math.Round(cw2)), int(math.Round(ch2)), int(math.Round(cx)), int(math.Round(cy)),
		}
		if i > 0 && cur == prev {
			continue
		}
		fmt.Fprintf(&b, "%.4f overlay@cur x %d, overlay@cur y %d, crop@cam w %d, crop@cam h %d, crop@cam x %d, crop@cam y %d;\n",
			math.Max(0, t-0.5/v.fps), cur[0], cur[1], cur[2], cur[3], cur[4], cur[5])
		prev = cur
	}
	if err := os.WriteFile(cmdPath, []byte(b.String()), 0o644); err != nil {
		return err
	}

	graph := fmt.Sprintf(
		"[0:v]setpts=PTS-STARTPTS,sendcmd=f='%s',pad=%d:%d:%d:%d[s];"+
			"[s][1:v]overlay=0:0[f];"+
			"[2:v]scale=%d:%d:flags=lanczos[c];"+
			"[f][c]overlay@cur=x=-1000:y=-1000[m];"+
			"[m]crop@cam=w=%d:h=%d:x=0:y=0,scale=%d:%d:flags=bicubic,format=yuv420p[out]",
		cmdPath, sw, sh, pad, pad, cw, ch, sw, sh, ow, oh)
	args := []string{"-hide_banner", "-v", "error", "-stats", "-y",
		"-i", in, "-i", framePNG, "-i", cursorPNG,
		"-filter_complex", graph, "-map", "[out]", "-an",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", strconv.Itoa(opt.crf), "-g", "60",
		"-r", strconv.FormatFloat(v.fps, 'f', -1, 64), "-movflags", "+faststart", outPath}
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return nil
}
