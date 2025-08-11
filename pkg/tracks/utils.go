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
