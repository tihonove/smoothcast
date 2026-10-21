package main

// Auto-zoom on clicks: a port from Capptivo (MIT, © 2026 idboussadel, see LICENSE.capptivo):
//   src/windows/editor/lib/zoomSuggestionUtils.ts — click series into zoom windows;
//   src-tauri/engine/src/timeline/zoom.rs — follow-cursor mode and the camera.
// The fixed-rect mode and the webcam face were not ported.

import (
	"math"
	"sort"
)

const (
	clickMergeGap   = 2.5 // clicks closer than this form one series
	clickPad        = 0.5 // margin before the first and after the last click
	minZoomDuration = 1.2

	zoomSampleFPS = 120.0

	followCenterTau = 0.55
	followZeta      = 1.05
	followMaxVel    = 3.4
	safeZoneInner   = 0.5
	clickHold       = 0.4
	followPanSnap   = 2e-5
	followVelSnap   = 0.028
)

type zoomFragment struct {
	start, end      float64
	scale           float64 // targetScale
	easeIn, easeOut float64
	damping         float64
}

type zoomKey struct{ t, x, y, scale float64 }

// suggestZooms is `buildClickZoomSuggestions` without reserving manual zooms.
// Unlike Capptivo, windows less than hold apart are merged: there they are
// suggestions for an editor, here it's the final video, and the camera shouldn't
// pull out for a couple of seconds only to zoom right back in.
func suggestZooms(clicks []sample, duration, scale, hold float64) []zoomFragment {
	var cs []sample
	for _, c := range clicks {
		if c.t >= 0 && c.t <= duration {
			cs = append(cs, c)
		}
	}
	if len(cs) == 0 {
		return nil
	}
	type cluster struct{ first, last float64 }
	var clusters []cluster
	cur := cluster{cs[0].t, cs[0].t}
	for _, c := range cs[1:] {
		if c.t-cur.last <= clickMergeGap {
			cur.last = math.Max(cur.last, c.t)
		} else {
			clusters = append(clusters, cur)
			cur = cluster{c.t, c.t}
		}
	}
	clusters = append(clusters, cur)

	var frags []zoomFragment
	for _, c := range clusters {
		start := math.Max(0, c.first-clickPad)
		end := math.Min(duration, c.last+clickPad)
		if end <= start {
			continue
		}
		if end-start < minZoomDuration {
			mid := (c.first + c.last) / 2
			start, end = mid-minZoomDuration/2, mid+minZoomDuration/2
			if start < 0 {
				end = math.Min(duration, end-start)
				start = 0
			}
			if end > duration {
				start = math.Max(0, start-(end-duration))
				end = duration
			}
		}
		if end-start < 0.05 {
			continue
		}
		if n := len(frags); n > 0 && start-frags[n-1].end < hold {
			frags[n-1].end = math.Max(frags[n-1].end, end)
			continue
		}
		// createDefaultZoomFragment + createSuggestedZoomFragment
		frags = append(frags, zoomFragment{start, end, scale, 1, 1, 4})
	}
	return frags
}

func easeOutCubic(t float64) float64 { return 1 - math.Pow(1-clamp(t, 0, 1), 3) }

// envelope is 0 → 1 → 0: zoom in, plateau, zoom out (`zoom_envelope`).
func (f zoomFragment) envelope(t float64) float64 {
	total := math.Max(1e-6, f.end-f.start)
	local := clamp(t-f.start, 0, total)
	in := clamp(f.easeIn, 0, total/2)
	out := clamp(f.easeOut, 0, total/2)
	if in > 0 && local < in {
		return easeOutCubic(local / in)
	}
	if out > 0 && local > total-out {
		return 1 - easeOutCubic(clamp((local-(total-out))/out, 0, 1))
	}
	return 1
}

// clampTarget keeps the center so the frame edge doesn't leave the recording.
func clampTarget(p vec2, scale float64) vec2 {
	half := 0.5 / math.Max(1, scale)
	if half >= 1-half-1e-9 {
		return vec2{0.5, 0.5}
	}
	return vec2{clamp(p.x, half, 1-half), clamp(p.y, half, 1-half)}
}

// recenter moves the focus along an axis only when the cursor leaves the inner zone.
func recenter(cur, cursor vec2, scale float64) vec2 {
	half := 0.5 / math.Max(1, scale)
	safe := half * clamp(safeZoneInner, 0.2, 0.95)
	next := cur
	if cursor.x < cur.x-safe || cursor.x > cur.x+safe {
		next.x = cursor.x
	}
	if cursor.y < cur.y-safe || cursor.y > cur.y+safe {
		next.y = cursor.y
	}
	return clampTarget(next, scale)
}

type motion struct{ x, y, vx, vy float64 }

// stepFollow is an overdamped camera spring with overshoot clamping
// (`step_spring_motion` + `step_follow_spring`).
func stepFollow(m motion, target vec2, dt, stiffness, scale float64) motion {
	dt = math.Max(1e-6, math.Min(dt, 0.05))
	w := stiffness
	ax := w*w*(target.x-m.x) - 2*followZeta*w*m.vx
	ay := w*w*(target.y-m.y) - 2*followZeta*w*m.vy
	n := motion{vx: clamp(m.vx+ax*dt, -followMaxVel, followMaxVel), vy: clamp(m.vy+ay*dt, -followMaxVel, followMaxVel)}
	n.x = clamp(m.x+n.vx*dt, 0, 1)
	n.y = clamp(m.y+n.vy*dt, 0, 1)
	if (m.x <= target.x && n.x > target.x) || (m.x >= target.x && n.x < target.x) {
		n.x, n.vx = target.x, 0
	}
	if (m.y <= target.y && n.y > target.y) || (m.y >= target.y && n.y < target.y) {
		n.y, n.vy = target.y, 0
	}
	if math.Hypot(target.x-n.x, target.y-n.y) < followPanSnap && math.Hypot(n.vx, n.vy) < followVelSnap {
		n = motion{x: target.x, y: target.y}
	}
	c := clampTarget(vec2{n.x, n.y}, scale)
	n.x, n.y = c.x, c.y
	return n
}

// keyframes integrates one follow-cursor fragment (`compute_zoom_keyframes`).
func (f zoomFragment) keyframes(tr *track) []zoomKey {
	n := int(math.Ceil((f.end-f.start)*zoomSampleFPS)) + 1
	keys := make([]zoomKey, 0, n)
	dt := 1 / zoomSampleFPS
	d := f.damping
	if d < 1 {
		d = 4
	}
	tau := followCenterTau * clamp(d, 1, 20) / 4
	stiffness := clamp(2.95/math.Max(tau, 0.065), 4.2, 30)

	full := math.Max(1, f.scale)
	outStart := f.end - clamp(f.easeOut, 0, (f.end-f.start)/2)
	m := motion{x: 0.5, y: 0.5}
	committed := vec2{0.5, 0.5}
	seed, _ := tr.at(f.start)
	var anchor *vec2

	for i := 0; i < n; i++ {
		t := f.start + float64(i)/zoomSampleFPS
		env := f.envelope(t)
		scale := 1 + (full-1)*env
		inEaseOut := f.easeOut > 0 && t > outStart && env < 1
		cursor, _ := tr.at(t)
		switch {
		case inEaseOut:
			if anchor == nil {
				anchor = &vec2{m.x, m.y}
			}
			p := clampTarget(vec2{0.5 + (anchor.x-0.5)*env, 0.5 + (anchor.y-0.5)*env}, scale)
			m = motion{x: p.x, y: p.y}
			committed = p
		case env < 1:
			// While zooming in, the focus stays on the start point; only the scale changes.
			p := clampTarget(seed, full)
			m = motion{x: p.x, y: p.y}
			committed = p
		default:
			target, k := recenter(committed, cursor, scale), stiffness
			if c, ok := activeClick(tr.clicks, t); ok {
				target, k = clampTarget(vec2{c.x, c.y}, scale), stiffness*1.35
			}
			committed = target
			m = stepFollow(m, target, dt, k, scale)
		}
		cs := scale
		if env < 1 && !inEaseOut {
			cs = full
		}
		p := clampTarget(vec2{m.x, m.y}, cs)
		m.x, m.y = p.x, p.y
		keys = append(keys, zoomKey{t, p.x, p.y, scale})
	}
	return keys
}

func activeClick(clicks []sample, t float64) (sample, bool) {
	for i := len(clicks) - 1; i >= 0; i-- {
		el := t - clicks[i].t
		if el < 0 {
			continue
		}
		if el <= clickHold {
			return clicks[i], true
		}
		break
	}
	return sample{}, false
}

type camera struct {
	frags []zoomFragment
	keys  [][]zoomKey
}

func newCamera(frags []zoomFragment, tr *track) *camera {
	c := &camera{frags: frags}
	for _, f := range frags {
		c.keys = append(c.keys, f.keyframes(tr))
	}
	return c
}

// at returns the focus (recording fractions), current and target scale at time t.
func (c *camera) at(t float64) (focus vec2, scale, target float64) {
	for i, f := range c.frags {
		if t < f.start || t > f.end {
			continue
		}
		k := c.keys[i]
		j := sort.Search(len(k), func(j int) bool { return k[j].t > t })
		switch {
		case j == 0:
			return vec2{k[0].x, k[0].y}, k[0].scale, f.scale
		case j == len(k):
			l := k[len(k)-1]
			return vec2{l.x, l.y}, l.scale, f.scale
		}
		a, b := k[j-1], k[j]
		u := (t - a.t) / math.Max(1e-9, b.t-a.t)
		return vec2{a.x + (b.x-a.x)*u, a.y + (b.y-a.y)*u}, a.scale + (b.scale-a.scale)*u, f.scale
	}
	return vec2{0.5, 0.5}, 1, 1
}

// rect is the visible part of the sw×sh scene where the recording sits at video
// (`camera_transform`, converted into a rectangle for crop).
func (c *camera) rect(t, sw, sh float64, video [4]float64) (x, y, w, h float64) {
	focus, scale, target := c.at(t)
	scale = math.Max(1, scale)
	if scale <= 1+1e-6 {
		return 0, 0, sw, sh
	}
	fx := video[0] + focus.x*video[2]
	fy := video[1] + focus.y*video[3]
	target = math.Max(scale, target)
	progress := 1.0
	if target > 1+1e-6 {
		progress = clamp((scale-1)/(target-1), 0, 1)
	}
	tx := progress * (sw/2 - fx*target)
	ty := progress * (sh/2 - fy*target)
	w, h = sw/scale, sh/scale
	return clamp(-tx/scale, 0, sw-w), clamp(-ty/scale, 0, sh-h), w, h
}
