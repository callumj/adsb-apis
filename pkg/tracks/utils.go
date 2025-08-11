package tracks

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// --- replace the stubbed functions above with these ---

func loadJSON(ctx context.Context, c *http.Client, url, ua string) (*aircraftJSON, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var aj aircraftJSON
	if err := json.Unmarshal(b, &aj); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &aj, nil
}

func getTile(ctx context.Context, c *http.Client, tmpl, ua, cache string, z, x, y int) (image.Image, error) {
	url := strings.NewReplacer("{z}", fmt.Sprint(z), "{x}", fmt.Sprint(x), "{y}", fmt.Sprint(y)).Replace(tmpl)
	if cache != "" {
		p := filepath.Join(cache, fmt.Sprintf("%d/%d/%d.png", z, x, y))
		if f, err := os.Open(p); err == nil {
			defer f.Close()
			return png.Decode(f)
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("User-Agent", ua)
		resp, err := c.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("tile status %s", resp.Status)
		}
		buf, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, buf, 0o644)
		return png.Decode(bytes.NewReader(buf))
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("tile status %s", resp.Status)
	}
	return png.Decode(resp.Body)
}

// Web Mercator, drawing, and small utils
func llToPixels(lat, lon float64, zoom int) (px, py float64) {
	lat = clamp(lat, -85.05112878, 85.05112878)
	pow2 := 1 << uint(zoom)
	s := float64(256 * pow2) // ✅ shift is done in int space

	x := (lon + 180.0) / 360.0
	sin := math.Sin(lat * math.Pi / 180.0)
	y := 0.5 - math.Log((1+sin)/(1-sin))/(4*math.Pi)
	return x * s, y * s
}

func wrapTileXY(tx, ty, zoom int) (int, int) {
	n := 1 << uint(zoom)
	for tx < 0 {
		tx += n
	}
	for tx >= n {
		tx -= n
	}
	if ty < 0 {
		ty = 0
	}
	if ty >= n {
		ty = n - 1
	}
	return tx, ty
}
func colorForHex(hex string) color.RGBA {
	h := sha1.Sum([]byte(hex))
	seed := int64(int(h[0])<<24 | int(h[1])<<16 | int(h[2])<<8 | int(h[3]))
	r := rand.New(rand.NewSource(seed))
	return color.RGBA{uint8(30 + r.Intn(200)), uint8(30 + r.Intn(200)), uint8(30 + r.Intn(200)), 255}
}
func setPixel(img *image.RGBA, x, y int, c color.RGBA) {
	if image.Pt(x, y).In(img.Bounds()) {
		img.Set(x, y, c)
	}
}
func line(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := int(math.Abs(float64(x1 - x0)))
	dy := -int(math.Abs(float64(y1 - y0)))
	sx := 1
	if x0 >= x1 {
		sx = -1
	}
	sy := 1
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	for {
		setPixel(img, x0, y0, c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}
func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				setPixel(img, cx+x, cy+y, c)
			}
		}
	}
}
func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
}
func drawLabel(img *image.RGBA, x, y int, text string, fg, bg color.RGBA) {
	w := textWidth(text)
	fillRect(img, x-3, y-12, w+6, 15, bg)
	col := image.NewUniform(fg)
	d := &font.Drawer{Dst: img, Src: col, Face: basicfont.Face7x13, Dot: fixed.P(x, y)}
	d.DrawString(text)
}
func textWidth(s string) int { return len(s) * 7 }
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// toOneBit converts an RGBA image to a 1-bit, 2-color paletted image.
// If dither is true, uses Floyd–Steinberg error diffusion; otherwise a hard threshold.
// "invert" swaps black and white.
func toOneBit(src *image.RGBA, threshold uint8, dither bool, invert bool) *image.Paletted {
	bounds := src.Bounds()
	var pal color.Palette
	if invert {
		pal = color.Palette{color.White, color.Black} // index 0=white, 1=black
	} else {
		pal = color.Palette{color.Black, color.White} // index 0=black, 1=white
	}
	dst := image.NewPaletted(bounds, pal)

	w, h := bounds.Dx(), bounds.Dy()

	// Error buffer for FS dithering (per pixel luminance error)
	if dither {
		errBuf := make([]float64, w*h)

		at := func(x, y int) int { return y*w + x }

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				// Luminance (Rec. 601)
				r, g, b, _ := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
				lum += errBuf[at(x, y)] // add propagated error

				var outIdx uint8
				var quant float64
				if invert {
					// quantize to white(255) or black(0) but swapped mapping
					if lum >= 128 {
						outIdx = 0 // white index
						quant = 255.0
					} else {
						outIdx = 1 // black index
						quant = 0.0
					}
				} else {
					if lum >= 128 {
						outIdx = 1 // white index
						quant = 255.0
					} else {
						outIdx = 0 // black index
						quant = 0.0
					}
				}
				// Write pixel
				dst.SetColorIndex(bounds.Min.X+x, bounds.Min.Y+y, outIdx)

				// Compute error and diffuse
				err := lum - quant
				// Distribute: 7/16 to (x+1, y), 3/16 to (x-1, y+1), 5/16 to (x, y+1), 1/16 to (x+1, y+1)
				if x+1 < w {
					errBuf[at(x+1, y)] += err * 7.0 / 16.0
				}
				if y+1 < h {
					if x > 0 {
						errBuf[at(x-1, y+1)] += err * 3.0 / 16.0
					}
					errBuf[at(x, y+1)] += err * 5.0 / 16.0
					if x+1 < w {
						errBuf[at(x+1, y+1)] += err * 1.0 / 16.0
					}
				}
			}
		}
		return dst
	}

	// No dithering: hard threshold
	thr := float64(threshold)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
			var idx uint8
			if invert {
				if lum >= thr {
					idx = 0 // white
				} else {
					idx = 1 // black
				}
			} else {
				if lum >= thr {
					idx = 1 // white
				} else {
					idx = 0 // black
				}
			}
			dst.SetColorIndex(bounds.Min.X+x, bounds.Min.Y+y, idx)
		}
	}
	return dst
}

func strokeLine(img *image.RGBA, x0, y0, x1, y1, w int, c color.RGBA) {
	if w <= 1 {
		line(img, x0, y0, x1, y1, c)
		return
	}
	dx := x1 - x0
	dy := y1 - y0
	steps := int(math.Hypot(float64(dx), float64(dy)))
	if steps < 1 {
		steps = 1
	}
	r := w / 2
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + t*float64(dx)))
		y := int(math.Round(float64(y0) + t*float64(dy)))
		fillCircle(img, x, y, r, c)
	}
}

func toGray(src image.Image) *image.Gray {
	b := src.Bounds()
	dst := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			// luminance (sRGB): 0.2126 R + 0.7152 G + 0.0722 B
			yv := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			dst.SetGray(x, y, color.Gray{Y: uint8((yv/65535.0)*255.0 + 0.5)})
		}
	}
	return dst
}

func adjustGray(g *image.Gray, contrast, gamma float64, invert bool) *image.Gray {
	b := g.Bounds()
	dst := image.NewGray(b)
	invGamma := 1.0
	if gamma > 0 && gamma != 1.0 {
		invGamma = 1.0 / gamma
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := g.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x++ {
			v := float64(g.Pix[i]) / 255.0
			// gamma
			if gamma != 1.0 {
				v = math.Pow(v, invGamma)
			}
			// contrast around mid-gray
			v = (v-0.5)*contrast + 0.5
			if invert {
				v = 1.0 - v
			}
			if v < 0 {
				v = 0
			} else if v > 1 {
				v = 1
			}
			dst.Pix[i] = uint8(v*255.0 + 0.5)
			i++
		}
	}
	return dst
}

func thresholdBW(g *image.Gray, thresh uint8) *image.Paletted {
	b := g.Bounds()
	pal := color.Palette{color.Black, color.White}
	dst := image.NewPaletted(b, pal)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := g.PixOffset(b.Min.X, y)
		j := dst.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x++ {
			if g.Pix[i] >= thresh {
				dst.Pix[j] = 1 // white
			} else {
				dst.Pix[j] = 0 // black
			}
			i++
			j++
		}
	}
	return dst
}

func floydSteinbergBW(g *image.Gray) *image.Paletted {
	b := g.Bounds()
	w, h := b.Dx(), b.Dy()
	buf := make([]float64, w*h)
	// seed with grayscale values 0..255
	for y := 0; y < h; y++ {
		copy(buf[y*w:(y+1)*w], bytesToFloats(g.Pix[g.PixOffset(b.Min.X, b.Min.Y+y):g.PixOffset(b.Min.X, b.Min.Y+y)+w]))
	}
	pal := color.Palette{color.Black, color.White}
	dst := image.NewPaletted(b, pal)

	at := func(x, y int) *float64 { return &buf[y*w+x] }

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			old := *at(x, y)
			newv := 0.0
			idx := uint8(0)
			if old >= 128.0 {
				newv = 255.0
				idx = 1
			}
			dst.Pix[dst.PixOffset(b.Min.X+x, b.Min.Y+y)] = idx
			err := old - newv
			if x+1 < w {
				*at(x+1, y) += err * 7 / 16
			}
			if y+1 < h {
				if x > 0 {
					*at(x-1, y+1) += err * 3 / 16
				}
				*at(x, y+1) += err * 5 / 16
				if x+1 < w {
					*at(x+1, y+1) += err * 1 / 16
				}
			}
		}
	}
	return dst
}

func bytesToFloats(p []uint8) []float64 {
	f := make([]float64, len(p))
	for i := range p {
		f[i] = float64(p[i])
	}
	return f
}

func quantizeGrayLevels(g *image.Gray, levels []uint8) *image.Paletted {
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
	pal := make(color.Palette, len(levels))
	for i, v := range levels {
		pal[i] = color.Gray{Y: v}
	}
	b := g.Bounds()
	dst := image.NewPaletted(b, pal)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := g.PixOffset(b.Min.X, y)
		j := dst.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x++ {
			v := g.Pix[i]
			// nearest level
			best := 0
			bestd := 999
			for li, lv := range levels {
				d := int(v) - int(lv)
				if d < 0 {
					d = -d
				}
				if d < bestd {
					bestd = d
					best = li
				}
			}
			dst.Pix[j] = uint8(best)
			i++
			j++
		}
	}
	return dst
}
