package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// curlog log: time is CLOCK_REALTIME microseconds, coordinates are region pixels.
type logLine struct {
	V      int     `json:"v"`
	T      int64   `json:"t"`
	X      *int    `json:"x"`
	Y      *int    `json:"y"`
	Ev     string  `json:"ev"`
	Btn    string  `json:"btn"`
	Region []int   `json:"region"`
	Scale  float64 `json:"scale"`
}

type sample struct{ t, x, y float64 }

// track is the cursor in seconds from the first video frame and in region
// fractions 0–1, as the Capptivo formulas expect it.
type track struct {
	w, h    float64
	samples []sample
	clicks  []sample
	stop    int64 // when curlog was stopped, µs
}

func loadTrack(path string) (*track, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var tr track
	var raw []logLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l logLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", path, err)
		}
		if l.V != 0 {
			if len(l.Region) != 4 {
				return nil, 0, fmt.Errorf("%s: header without region", path)
			}
			tr.w, tr.h = float64(l.Region[2]), float64(l.Region[3])
			continue
		}
		if l.Ev == "stop" {
			tr.stop = l.T
		}
		raw = append(raw, l)
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	if tr.w == 0 {
		return nil, 0, fmt.Errorf("%s: no header", path)
	}
	var first int64
	if len(raw) > 0 {
		first = raw[0].T
	}
	tr.samples = make([]sample, 0, len(raw))
	for _, l := range raw {
		if l.Ev == "" && l.X != nil {
			tr.samples = append(tr.samples, sample{float64(l.T), float64(*l.X), float64(*l.Y)})
		}
	}
	for _, l := range raw {
		if l.Ev == "down" && l.Btn == "left" {
			tr.clicks = append(tr.clicks, sample{t: float64(l.T)})
		}
	}
	return &tr, first, nil
}

// rebase converts time to seconds from t0 (µs) and coordinates to region
// fractions; clicks get the cursor position at the moment of the press.
func (tr *track) rebase(t0 int64) {
	for i := range tr.samples {
		s := &tr.samples[i]
		s.t = (s.t - float64(t0)) / 1e6
		s.x /= tr.w
		s.y /= tr.h
	}
	for i := range tr.clicks {
		c := &tr.clicks[i]
		c.t = (c.t - float64(t0)) / 1e6
		if p, ok := tr.at(c.t); ok {
			c.x, c.y = p.x, p.y
		}
	}
	sort.Slice(tr.clicks, func(i, j int) bool { return tr.clicks[i].t < tr.clicks[j].t })
}

// at linearly interpolates the recorded track (`sample_cursor_at_time`).
func (tr *track) at(t float64) (vec2, bool) {
	s := tr.samples
	if len(s) == 0 {
		return vec2{}, false
	}
	if t <= s[0].t {
		return vec2{s[0].x, s[0].y}, true
	}
	last := s[len(s)-1]
	if t >= last.t {
		return vec2{last.x, last.y}, true
	}
	i := sort.Search(len(s), func(i int) bool { return s[i].t > t }) - 1
	a, b := s[i], s[i+1]
	p := clamp((t-a.t)/max(1e-6, b.t-a.t), 0, 1)
	return vec2{a.x + (b.x-a.x)*p, a.y + (b.y-a.y)*p}, true
}
