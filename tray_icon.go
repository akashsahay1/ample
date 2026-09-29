package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Placeholder tray icons, drawn procedurally so the build does not depend on
// asset files. The lead may swap these for assets/icons/tray-*.ico (Agent D).

type seg struct{ x1, y1, x2, y2, w float64 }

func distSeg(px, py float64, s seg) float64 {
	dx, dy := s.x2-s.x1, s.y2-s.y1
	t := ((px-s.x1)*dx + (py-s.y1)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	cx, cy := s.x1+t*dx, s.y1+t*dy
	return math.Hypot(px-cx, py-cy)
}

func blend(dst color.NRGBA, c color.NRGBA, a float64) color.NRGBA {
	if a <= 0 {
		return dst
	}
	a = math.Min(1, a) * float64(c.A) / 255
	da := float64(dst.A) / 255
	oa := a + da*(1-a)
	if oa == 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 {
		return uint8((float64(s)*a + float64(d)*da*(1-a)) / oa)
	}
	return color.NRGBA{mix(c.R, dst.R), mix(c.G, dst.G), mix(c.B, dst.B), uint8(oa * 255)}
}

// roundRectCov returns coverage of a rounded rect in 256-space at point (x,y).
func roundRectCov(x, y, x0, y0, x1, y1, r, px float64) float64 {
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	d := math.Hypot(x-cx, y-cy) - r
	return 0.5 - d/px
}

func drawIcon(size int, status color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := 256 / float64(size)
	bg := color.NRGBA{0x2A, 0x2F, 0x39, 255}
	fg := color.NRGBA{0xF5, 0xF3, 0xEF, 255}
	bars := []struct {
		x, y, w float64
		c       color.NRGBA
	}{
		{113, 129, 30, color.NRGBA{0xE8, 0x62, 0x2C, 255}},
		{104, 151, 48, color.NRGBA{0x9A, 0xA3, 0xFF, 255}},
		{96, 173, 64, color.NRGBA{0x3F, 0xA9, 0xC9, 255}},
	}
	strokes := []seg{{70, 200, 128, 56, 26}, {128, 56, 186, 200, 26}}
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			x, y := (float64(px)+0.5)*scale, (float64(py)+0.5)*scale
			c := img.NRGBAAt(px, py)
			c = blend(c, bg, roundRectCov(x, y, 0, 0, 256, 256, 58, scale))
			for _, s := range strokes {
				c = blend(c, fg, 0.5-(distSeg(x, y, s)-s.w/2)/scale)
			}
			for _, b := range bars {
				c = blend(c, b.c, roundRectCov(x, y, b.x, b.y, b.x+b.w, b.y+12, 6, scale))
			}
			// status dot, bottom-right
			if status.A > 0 {
				d := math.Hypot(x-206, y-206)
				c = blend(c, bg, 0.5-(d-50)/scale)
				c = blend(c, status, 0.5-(d-38)/scale)
			}
			img.SetNRGBA(px, py, c)
		}
	}
	return img
}

// buildICO encodes PNG-in-ICO with the given sizes.
func buildICO(status color.NRGBA, sizes ...int) []byte {
	var pngs [][]byte
	for _, s := range sizes {
		var buf bytes.Buffer
		_ = png.Encode(&buf, drawIcon(s, status))
		pngs = append(pngs, buf.Bytes())
	}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		w := uint8(s)
		if s >= 256 {
			w = 0
		}
		out.Write([]byte{w, w, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))
		_ = binary.Write(&out, binary.LittleEndian, uint16(32))
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(pngs[i])))
		_ = binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return out.Bytes()
}

var (
	iconRunning = func() []byte { return buildICO(color.NRGBA{0x3F, 0xB5, 0x7A, 255}, 16, 24, 32, 48) }
	iconStopped = func() []byte { return buildICO(color.NRGBA{0x9A, 0x9E, 0xA8, 255}, 16, 24, 32, 48) }
	iconPartial = func() []byte { return buildICO(color.NRGBA{0xF2, 0xB5, 0x44, 255}, 16, 24, 32, 48) }
)
