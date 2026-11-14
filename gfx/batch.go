package gfx

import (
	"math"

	"github.com/tesinclair/hexdm/settings"
)

// rgba is a colour that has not been multiplied by its alpha yet.
type rgba struct{ r, g, b, a float32 }

func fromConfig(c settings.Color) rgba { return rgba{c[0], c[1], c[2], 1} }

// alpha returns the colour made more transparent by a.
func (c rgba) alpha(a float32) rgba {
	c.a *= a
	return c
}

func mix(a, b rgba, t float32) rgba {
	return rgba{lerp(a.r, b.r, t), lerp(a.g, b.g, t), lerp(a.b, b.b, t), lerp(a.a, b.a, t)}
}

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func clamp01(v float32) float32 { return min(max(v, 0), 1) }

// ease goes from 0 to 1 as v does, fast at first and settling at the end.
func ease(v float32) float32 {
	v = 1 - clamp01(v)
	return 1 - v*v*v
}

func smooth(v float32) float32 {
	v = clamp01(v)
	return v * v * (3 - 2*v)
}

func exp(v float32) float32  { return float32(math.Exp(float64(v))) }
func sin(v float32) float32  { return float32(math.Sin(float64(v))) }
func sqrt(v float32) float32 { return float32(math.Sqrt(float64(v))) }

// What the fragment shader draws in a quad. Must match batch.frag.
const (
	kindGlyph = 0
	kindBox   = 1
	kindRing  = 2
	kindHex   = 3
)

const floatsPerVert = 16

const tau = 2 * math.Pi

// batch collects everything drawn on top of the background in a frame. It
// is sent to the GPU in one go.
type batch struct {
	v []float32
}

func (b *batch) reset() { b.v = b.v[:0] }

func (b *batch) verts() int { return len(b.v) / floatsPerVert }

// quad adds a rectangle from (x0,y0) to (x1,y1) in pixels. The l values are
// what the shader sees as its position at those two corners.
func (b *batch) quad(x0, y0, x1, y1, lx0, ly0, lx1, ly1, hw, hh float32, c rgba, kind, cut, stroke, glow, e0, e1 float32) {
	if c.a <= 0.002 {
		return
	}
	corner := func(x, y, lx, ly float32) {
		b.v = append(b.v, x, y, lx, ly, hw, hh, c.r, c.g, c.b, c.a, kind, cut, stroke, glow, e0, e1)
	}
	corner(x0, y0, lx0, ly0)
	corner(x1, y0, lx1, ly0)
	corner(x1, y1, lx1, ly1)
	corner(x0, y0, lx0, ly0)
	corner(x1, y1, lx1, ly1)
	corner(x0, y1, lx0, ly1)
}

// shape adds a quad big enough for a shape of the given half size and its
// glow, centred on (cx,cy).
func (b *batch) shape(cx, cy, hw, hh, reach float32, c rgba, kind, cut, stroke, glow, e0, e1 float32) {
	pad := glow*5 + 2
	rx, ry := reach+pad, reach+pad
	if kind == kindBox {
		rx, ry = hw+pad, hh+pad
	}
	b.quad(cx-rx, cy-ry, cx+rx, cy+ry, -rx, -ry, rx, ry, hw, hh, c, kind, cut, stroke, glow, e0, e1)
}

// box is a rectangle with its corners cut by cut pixels. A stroke of zero
// fills it.
func (b *batch) box(cx, cy, hw, hh, cut, stroke, glow float32, c rgba) {
	b.shape(cx, cy, hw, hh, 0, c, kindBox, cut, stroke, glow, 0, tau)
}

// ring is a circle of radius r. A stroke of zero fills it.
func (b *batch) ring(cx, cy, r, stroke, glow float32, c rgba) {
	b.shape(cx, cy, r, r, r, c, kindRing, 0, stroke, glow, 0, tau)
}

// arc is part of a ring, from the angle start and sweeping clockwise (angles
// in radians, zero pointing right).
func (b *batch) arc(cx, cy, r, stroke, glow, start, sweep float32, c rgba) {
	start = float32(math.Mod(float64(start), tau))
	b.shape(cx, cy, r, r, r, c, kindRing, 0, stroke, glow, start, sweep)
}

// hex is a hexagon, point up. r is the distance from its centre to a side.
func (b *batch) hex(cx, cy, r, stroke, glow float32, c rgba) {
	b.shape(cx, cy, r, r, r*1.16, c, kindHex, 0, stroke, glow, 0, tau)
}
