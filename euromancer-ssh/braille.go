// Braille rendering of post attachments: the dithered B/W images are
// rescaled to the braille dot grid (2×4 dots per cell, near-square pitch)
// and re-dithered with Floyd–Steinberg. Light areas become raised dots —
// ink on the dark terminal, the image turned into type.
package main

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// dot bit masks of the braille cell, [row][column]
var brailleDots = [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

func brailleRender(path string, wCells int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}

	src := img.Bounds()
	pw := wCells * 2
	ph := src.Dy() * pw / src.Dx()
	gray := image.NewGray(image.Rect(0, 0, pw, ph))
	xdraw.CatmullRom.Scale(gray, gray.Bounds(), img, src, xdraw.Src, nil)

	// Floyd–Steinberg down to one bit per dot
	buf := make([]float64, pw*ph)
	for i, v := range gray.Pix {
		buf[i] = float64(v)
	}
	bits := make([]bool, pw*ph)
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			i := y*pw + x
			var nv float64
			if buf[i] >= 128 {
				nv, bits[i] = 255, true
			}
			qe := buf[i] - nv
			if x+1 < pw {
				buf[i+1] += qe * 7 / 16
			}
			if y+1 < ph {
				if x > 0 {
					buf[i+pw-1] += qe * 3 / 16
				}
				buf[i+pw] += qe * 5 / 16
				if x+1 < pw {
					buf[i+pw+1] += qe * 1 / 16
				}
			}
		}
	}

	var b strings.Builder
	for cy := 0; cy < ph; cy += 4 {
		for cx := 0; cx < pw; cx += 2 {
			cell := rune(0x2800)
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					if x, y := cx+dx, cy+dy; y < ph && bits[y*pw+x] {
						cell |= brailleDots[dy][dx]
					}
				}
			}
			b.WriteRune(cell)
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}
