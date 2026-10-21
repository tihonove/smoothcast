package main

// Smooth cursor: a port of src-tauri/engine/src/timeline/cursor.rs from Capptivo
// (MIT, © 2026 idboussadel, see LICENSE.capptivo). Only the core is taken — an
// analytically stepped spring, snapping to the click point and a reset after a
// gap. Sway, trail and auto-hide are not ported yet.

import "math"

type vec2 struct{ x, y float64 }

const (
	clickFreeze = 0.1  // CURSOR_CLICK_FREEZE_S: around a press the cursor stays exactly in place
	seekGap     = 0.12 // CURSOR_SEEK_GAP_S: a longer gap without frames is a jump, don't chase
)

func clamp(v, lo, hi float64) float64 { return math.Min(hi, math.Max(lo, v)) }

func smoothstep01(x float64) float64 {
	t := clamp(x, 0, 1)
	return t * t * (3 - 2*t)
}

type springConfig struct{ stiffness, damping, mass, restDelta, restSpeed float64 }

type springState struct {
	value, velocity float64
	initialized     bool
}

// cursorSpringConfig maps the 0–1 "smoothness" slider to spring parameters
// (`cursor_spring_config`); Capptivo defaults to 0.45.
func cursorSpringConfig(smoothness float64) springConfig {
	s := clamp(smoothness, 0, 1)
	if s <= 0.02 {
		return springConfig{1200, 120, 1, 0.0001, 0.001}
	}
	return springConfig{760 - s*480, 36 + s*28, 0.85 + s*0.65, 0.0002, 0.01}
}

func clampDeltaSec(dt, fallback float64) float64 {
	if math.IsNaN(dt) || math.IsInf(dt, 0) || dt <= 0 {
		return fallback
	}
	return math.Min(0.08, math.Max(1.0/240, dt))
}

// springPosition is the position of a damped oscillator after t seconds.
func springPosition(t, target, delta, velocity, zeta, omega float64) float64 {
	switch {
	case zeta < 1:
		wd := omega * math.Sqrt(1-zeta*zeta)
		env := math.Exp(-zeta * omega * t)
		return target - env*(((velocity+zeta*omega*delta)/wd)*math.Sin(wd*t)+delta*math.Cos(wd*t))
	case zeta == 1:
		return target - math.Exp(-omega*t)*(delta+(velocity+omega*delta)*t)
	default:
		wd := omega * math.Sqrt(zeta*zeta-1)
		env := math.Exp(-zeta * omega * t)
		ft := math.Min(wd*t, 300)
		return target - env*((velocity+zeta*omega*delta)*math.Sinh(ft)+wd*delta*math.Cosh(ft))/wd
	}
}

// step is one analytic spring step (`step_spring_value`).
func (s *springState) step(target, dt float64, c springConfig) float64 {
	dt = clampDeltaSec(dt, 1.0/60)
	if !s.initialized || math.IsNaN(s.value) {
		*s = springState{target, 0, true}
		return s.value
	}
	if math.Abs(target-s.value) <= c.restDelta && math.Abs(s.velocity) <= c.restSpeed {
		s.value, s.velocity = target, 0
		return s.value
	}
	omega := math.Sqrt(c.stiffness / c.mass)
	zeta := c.damping / (2 * math.Sqrt(c.stiffness*c.mass))
	delta := target - s.value
	velocity := -s.velocity
	cur := springPosition(dt, target, delta, velocity, zeta, omega)
	if zeta >= 1 && ((s.value <= target && cur > target) || (s.value >= target && cur < target)) {
		s.value, s.velocity = target, 0
		return s.value
	}
	const eps = 1e-4
	vel := (springPosition(dt+eps, target, delta, velocity, zeta, omega) - cur) / eps
	if math.Abs(vel) <= c.restSpeed && math.Abs(target-cur) <= c.restDelta {
		s.value, s.velocity = target, 0
	} else {
		s.value, s.velocity = cur, vel
	}
	return s.value
}

func nearClick(clicks []sample, t float64) bool {
	for _, c := range clicks {
		if t >= c.t-clickFreeze && t <= c.t+clickFreeze {
			return true
		}
	}
	return false
}

// smoothCursor runs the spring over the frame grid (`CursorTimeline::integrate`)
// and returns the tip position for every frame, in region fractions.
func smoothCursor(tr *track, frames int, fps, smoothness float64) []vec2 {
	cfg := cursorSpringConfig(smoothness)
	out := make([]vec2, frames)
	var xs, ys springState
	last := math.NaN()
	for i := range out {
		t := float64(i) / fps
		target, ok := tr.at(t)
		if !ok {
			out[i] = vec2{0.5, 0.5}
			continue
		}
		target = vec2{clamp(target.x, 0, 1), clamp(target.y, 0, 1)}
		dt := 1.0 / fps
		if !math.IsNaN(last) {
			dt = clampDeltaSec(t-last, dt)
		}
		seeked := !math.IsNaN(last) && t-last > seekGap
		if !xs.initialized || seeked || nearClick(tr.clicks, t) || smoothness <= 0.02 {
			xs = springState{target.x, 0, true}
			ys = springState{target.y, 0, true}
		} else {
			xs.step(target.x, dt, cfg)
			ys.step(target.y, dt, cfg)
		}
		last = t
		out[i] = vec2{xs.value, ys.value}
	}
	return out
}
