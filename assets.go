package main

// Static scene images: the background with a shadow and a rounded window for
// the recording, and the cursor from the xcursor theme. All computed once; then
// ffmpeg just overlays them on every frame.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
)

// Recording shadow as in Capptivo: three layers, sigma/opacity given for a short
// side of 1080 at strength 100 (`recordingShadowPasses`, default 75).
var shadowLayers = []struct{ sigma, opacity float64 }{{48, 0.7}, {16, 0.5}, {8, 0.3}}

// frameImage draws the sw×sh scene: background, shadow and a transparent window
// with rounded corners where the recording goes. The recording sits under it.
func frameImage(bg *image.RGBA, sw, sh int, video [4]float64, radius, shadow float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sw, sh))
	copy(img.Pix, bg.Pix)

	cover := make([]float32, sw*sh)
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			cover[y*sw+x] = float32(roundRectCoverage(float64(x)+0.5, float64(y)+0.5, video, radius))
		}
	}

	strength := shadow / 100
	spread := float64(min(sw, sh)) / 1080 * strength
	density := strength
	if strength > 1 {
		density = 1 + (strength-1)*0.3
	}
	buf := make([]float32, sw*sh)
	for _, l := range shadowLayers {
		copy(buf, cover)
		gaussBlur(buf, sw, sh, l.sigma*spread)
		op := math.Min(0.85, l.opacity*density)
		for i, a := range buf {
			k := 1 - float64(a)*op // black on top: (1-α) of the background remains
			p := img.Pix[i*4 : i*4+3]
			for c := range p {
				p[c] = uint8(float64(p[c]) * k)
			}
		}
	}
	// The window: transparent over the recording. Go's RGBA is alpha-premultiplied,
	// so all four channels are multiplied.
	for i, a := range cover {
		k := 1 - float64(a)
		p := img.Pix[i*4 : i*4+4]
		for c := range p {
			p[c] = uint8(float64(p[c])*k + 0.5)
		}
	}
	return img
}

func roundRectCoverage(px, py float64, r [4]float64, radius float64) float64 {
	cx, cy := r[0]+r[2]/2, r[1]+r[3]/2
	qx := math.Abs(px-cx) - (r[2]/2 - radius)
	qy := math.Abs(py-cy) - (r[3]/2 - radius)
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
	return clamp(0.5-d, 0, 1)
}

// gaussBlur is three box-blur passes per axis, close to a Gaussian.
func gaussBlur(buf []float32, w, h int, sigma float64) {
	if sigma < 0.5 {
		return
	}
	r := int(math.Round(math.Sqrt(12*sigma*sigma/3+1)-1) / 2)
	if r < 1 {
		r = 1
	}
	tmp := make([]float32, max(w, h))
	line := make([]float32, max(w, h))
	for pass := 0; pass < 3; pass++ {
		for y := 0; y < h; y++ {
			row := buf[y*w : y*w+w]
			copy(line, row)
			boxLine(line[:w], tmp[:w], r)
			copy(row, tmp[:w])
		}
		for x := 0; x < w; x++ {
			for y := 0; y < h; y++ {
				line[y] = buf[y*w+x]
			}
			boxLine(line[:h], tmp[:h], r)
			for y := 0; y < h; y++ {
				buf[y*w+x] = tmp[y]
			}
		}
	}
}

func boxLine(in, out []float32, r int) {
	n := len(in)
	at := func(i int) float32 {
		if i < 0 || i >= n {
			return 0 // nothing beyond the scene edge
		}
		return in[i]
	}
	var sum float32
	for i := -r; i <= r; i++ {
		sum += at(i)
	}
	inv := 1 / float32(2*r+1)
	for i := 0; i < n; i++ {
		out[i] = sum * inv
		sum += at(i+r+1) - at(i-r)
	}
}

// gradient is the default background: a soft diagonal.
func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	a := [3]float64{0x3a, 0x2f, 0x7a}
	b := [3]float64{0x1c, 0x8a, 0xa8}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := (float64(x)/float64(w) + float64(y)/float64(h)) / 2
			img.SetRGBA(x, y, color.RGBA{
				uint8(a[0] + (b[0]-a[0])*t), uint8(a[1] + (b[1]-a[1])*t), uint8(a[2] + (b[2]-a[2])*t), 255})
		}
	}
	return img
}

// imageBackground is an image scaled and cropped to fill the scene, slightly
// blurred (blur 9 in Capptivo). Decoding jpg/webp/… is left to ffmpeg.
func imageBackground(path string, w, h int, tmp string) (*image.RGBA, error) {
	out := filepath.Join(tmp, "bg.png")
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,gblur=sigma=%g,format=rgba",
		w, h, w, h, float64(min(w, h))/1080*9)
	cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-i", path, "-frames:v", "1", "-vf", vf, out)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("background %s: %w", path, err)
	}
	f, err := os.Open(out)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(src.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, src.At(x, y))
		}
	}
	return img, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// xcursor reads the arrow from the cursor theme (as sway does: XCURSOR_THEME),
// picking the nominal size closest to size. Returns the image and the hotspot.
func xcursor(size int) (*image.RGBA, image.Point, error) {
	theme := os.Getenv("XCURSOR_THEME")
	if theme == "" {
		theme = "Adwaita"
	}
	home, _ := os.UserHomeDir()
	var data []byte
	for _, dir := range []string{home + "/.local/share/icons", home + "/.icons", "/usr/share/icons"} {
		if b, err := os.ReadFile(filepath.Join(dir, theme, "cursors", "left_ptr")); err == nil {
			data = b
			break
		}
	}
	if len(data) < 16 || string(data[:4]) != "Xcur" {
		return nil, image.Point{}, fmt.Errorf("no left_ptr in cursor theme %q", theme)
	}
	le := binary.LittleEndian
	ntoc := int(le.Uint32(data[12:]))
	best, bestPos := -1, 0
	for i := 0; i < ntoc; i++ {
		e := data[16+i*12:]
		if le.Uint32(e) != 0xfffd0002 { // image
			continue
		}
		nominal, pos := int(le.Uint32(e[4:])), int(le.Uint32(e[8:]))
		if best < 0 || abs(nominal-size) < abs(best-size) {
			best, bestPos = nominal, pos
		}
	}
	if best < 0 {
		return nil, image.Point{}, errors.New("cursor file has no images")
	}
	c := data[bestPos:]
	w, h := int(le.Uint32(c[16:])), int(le.Uint32(c[20:]))
	hot := image.Point{int(le.Uint32(c[24:])), int(le.Uint32(c[28:]))}
	px := c[36:]
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		// Little-endian ARGB, alpha already premultiplied — same as image.RGBA.
		img.Pix[i*4+0] = px[i*4+2]
		img.Pix[i*4+1] = px[i*4+1]
		img.Pix[i*4+2] = px[i*4+0]
		img.Pix[i*4+3] = px[i*4+3]
	}
	return img, hot, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
