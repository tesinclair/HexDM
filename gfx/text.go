package gfx

import (
	_ "embed"
	"fmt"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// The fonts are part of the binary, so the greeter looks the same whatever
// is installed. Both are under the SIL Open Font License (see fonts/).
var (
	//go:embed fonts/Orbitron-Medium.ttf
	orbitronMedium []byte
	//go:embed fonts/Orbitron-Bold.ttf
	orbitronBold []byte
	//go:embed fonts/JetBrainsMono-Regular.ttf
	jetbrainsMono []byte
)

type fontID int

const (
	fontDisplay fontID = iota
	fontDisplayBold
	fontMono
	fontCount
)

const atlasSize = 2048

type faceKey struct {
	font fontID
	px   int
}

type glyphKey struct {
	face faceKey
	r    rune
}

type glyph struct {
	u0, v0, u1, v1 float32
	x, y, w, h     float32 // where the bitmap sits relative to the pen
	advance        float32
}

// text draws strings from a glyph atlas that fills up as glyphs are first
// needed.
type text struct {
	fonts  [fontCount]*sfnt.Font
	faces  map[faceKey]font.Face
	glyphs map[glyphKey]glyph
	buf    sfnt.Buffer

	atlas  []byte
	dirty  bool // the atlas has changed since the GPU last saw it
	penX   int
	penY   int
	shelfH int
}

func newText() (*text, error) {
	t := &text{
		faces:  make(map[faceKey]font.Face),
		glyphs: make(map[glyphKey]glyph),
		atlas:  make([]byte, atlasSize*atlasSize),
		dirty:  true,
		penX:   1,
		penY:   1,
	}
	for id, data := range [fontCount][]byte{orbitronMedium, orbitronBold, jetbrainsMono} {
		f, err := opentype.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("embedded font %d: %w", id, err)
		}
		t.fonts[id] = f
	}
	return t, nil
}

func (t *text) face(key faceKey) font.Face {
	if f, ok := t.faces[key]; ok {
		return f
	}
	f, err := opentype.NewFace(t.fonts[key.font], &opentype.FaceOptions{
		Size:    float64(key.px),
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil
	}
	t.faces[key] = f
	return f
}

// glyph returns the rune's place in the atlas, drawing it there first if
// this is the first time it is needed at this size.
func (t *text) glyph(id fontID, px int, r rune) glyph {
	// The display font only has Latin. Anything else comes from the mono
	// font, which covers far more.
	if id != fontMono {
		if idx, err := t.fonts[id].GlyphIndex(&t.buf, r); err != nil || idx == 0 {
			id = fontMono
		}
	}
	key := glyphKey{faceKey{id, px}, r}
	if g, ok := t.glyphs[key]; ok {
		return g
	}

	var g glyph
	face := t.face(key.face)
	if face == nil {
		return g
	}
	dr, mask, maskp, advance, ok := face.Glyph(fixed.Point26_6{}, r)
	if !ok {
		return g
	}
	g.advance = float32(advance) / 64
	w, h := dr.Dx(), dr.Dy()
	if w > 0 && h > 0 && w < atlasSize-2 && h < atlasSize-2 {
		if t.penX+w+1 > atlasSize {
			t.penX = 1
			t.penY += t.shelfH + 1
			t.shelfH = 0
		}
		if t.penY+h+1 > atlasSize {
			// Full. Start again; this frame may show a few wrong
			// glyphs, the next one redraws them.
			clear(t.atlas)
			clear(t.glyphs)
			t.penX, t.penY, t.shelfH = 1, 1, 0
		}
		for y := 0; y < h; y++ {
			row := t.atlas[(t.penY+y)*atlasSize+t.penX:]
			for x := 0; x < w; x++ {
				_, _, _, a := mask.At(maskp.X+x, maskp.Y+y).RGBA()
				row[x] = byte(a >> 8)
			}
		}
		g.u0 = float32(t.penX) / atlasSize
		g.v0 = float32(t.penY) / atlasSize
		g.u1 = float32(t.penX+w) / atlasSize
		g.v1 = float32(t.penY+h) / atlasSize
		g.x, g.y = float32(dr.Min.X), float32(dr.Min.Y)
		g.w, g.h = float32(w), float32(h)
		t.penX += w + 1
		t.shelfH = max(t.shelfH, h)
		t.dirty = true
	}
	t.glyphs[key] = g
	return g
}

func pixels(size float32) int { return max(int(math.Round(float64(size))), 6) }

// width is how wide draw would make s.
func (t *text) width(id fontID, size float32, s string, tracking float32) float32 {
	px := pixels(size)
	var w float32
	n := 0
	for _, r := range s {
		w += t.glyph(id, px, r).advance
		n++
	}
	if n > 1 {
		w += tracking * float32(n-1)
	}
	return w
}

// draw writes s with its baseline starting at (x,y). tracking is extra
// space between letters.
func (t *text) draw(b *batch, id fontID, size, x, y float32, c rgba, s string, tracking float32) {
	px := pixels(size)
	x = float32(math.Round(float64(x)))
	y = float32(math.Round(float64(y)))
	for _, r := range s {
		g := t.glyph(id, px, r)
		if g.w > 0 {
			x0, y0 := x+g.x, y+g.y
			b.quad(x0, y0, x0+g.w, y0+g.h, g.u0, g.v0, g.u1, g.v1, 0, 0, c, kindGlyph, 0, 0, 0, 0, 0)
		}
		x += g.advance + tracking
	}
}

// centred writes s centred on x, with y the middle of its capital letters.
func (t *text) centred(b *batch, id fontID, size, x, y float32, c rgba, s string, tracking float32) float32 {
	w := t.width(id, size, s, tracking)
	t.draw(b, id, size, x-w/2, y+capHeight*float32(pixels(size))/2, c, s, tracking)
	return w
}

// right writes s ending at x, with its baseline at y.
func (t *text) right(b *batch, id fontID, size, x, y float32, c rgba, s string, tracking float32) float32 {
	w := t.width(id, size, s, tracking)
	t.draw(b, id, size, x-w, y, c, s, tracking)
	return w
}

// Height of a capital letter as a share of the font size. Close enough for
// both fonts to centre text on a line.
const capHeight = 0.72
