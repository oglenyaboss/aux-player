package main

import (
	"math"
	"sync"
)

// shoot brings the 4K source down to the 1080p picture.
func (f *film) shoot(n int, src, dst []byte) {
	var wg sync.WaitGroup
	const bands = 30
	for b := range bands {
		wg.Add(1)
		go func(b int) {
			defer wg.Done()
			for y := b * frameH / bands; y < (b+1)*frameH/bands; y++ {
				for x := range frameW {
					o := (2*y*srcW + 2*x) * 3
					d := (y*frameW + x) * 3
					for c := range 3 {
						sum := int(src[o+c]) + int(src[o+3+c]) + int(src[o+srcW*3+c]) + int(src[o+srcW*3+3+c])
						dst[d+c] = uint8(sum / 4) //nolint:gosec // G115: the mean of four bytes is a byte
					}
				}
			}
		}(b)
	}
	wg.Wait()
	f.subtitle(n, dst)
}

// subtitle types the caption of the moment over the picture, one size up from the
// terminal so a phone can read it.
func (f *film) subtitle(n int, dst []byte) {
	text, from, to := f.captionAt(n)
	if text == "" {
		return
	}
	rs := []rune("› " + text)
	shown := min(len(rs), 2+(n-from)*2/3)
	alpha := math.Min(1, float64(to-n)/8)
	bw := len(rs)*cellW + 2*cellW
	bh := cellH + 28
	x0 := (frameW - bw) / 2
	y0 := frameH - bh - 72
	for y := y0; y < y0+bh; y++ {
		for x := x0; x < x0+bw; x++ {
			o := (y*frameW + x) * 3
			for c := range 3 {
				dst[o+c] = uint8(float64(dst[o+c]) * (1 - 0.86*alpha))
			}
		}
	}
	for i, r := range rs[:shown] {
		g := f.glyphs[glyphKey{r, true}]
		if g == nil {
			continue
		}
		ink := [3]float64{240, 244, 244}
		if i == 0 {
			ink = [3]float64{125, 226, 200}
		}
		gx, gy := x0+cellW+i*cellW, y0+14
		for py := range cellH {
			for px := range cellW {
				a := float64(g[py*cellW+px]) / 255 * alpha
				if a == 0 {
					continue
				}
				o := ((gy+py)*frameW + gx + px) * 3
				for c := range 3 {
					dst[o+c] = uint8(float64(dst[o+c])*(1-a) + ink[c]*a)
				}
			}
		}
	}
}

func (f *film) captionAt(n int) (text string, from, to int) {
	if n >= f.last {
		from = f.last + frameOf(2.8)
		if n < from {
			return "", 0, 0
		}
		return f.e.EndCaption, from, f.end
	}
	for _, s := range f.e.Segments {
		if s.Caption != "" && n >= s.Start+frameOf(240/s.BPM/2) && n < s.Start+s.Frames {
			return s.Caption, s.Start + frameOf(240/s.BPM/2), s.Start + s.Frames
		}
	}
	return "", 0, 0
}
