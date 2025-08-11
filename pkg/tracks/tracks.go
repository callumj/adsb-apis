package tracks

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

/*
GET /tracks.png?center-lat=40.7128&center-lon=-74.0060&zoom=9&width=1600&height=1000&history=90

Optional query params (all have defaults):
- dump1090:      base URL (default http://localhost/dump1090-fa)
- data-path:     path under dump1090 (default /data)
- center-lat:    map center latitude (default 0)
- center-lon:    map center longitude (default 0)
- zoom:          slippy zoom (default 9)
- width:         image width px (default 1600)
- height:        image height px (default 1000)
- history:       how many history_N.json to read, 0..119 (default 90)
- include-latest:true/false include aircraft.json (default true)
- min-points:    minimum points for a track (default 2)
- label-last:    true/false label last position (default true)
- tile-url:      OSM tile template (default https://tile.openstreetmap.org/{z}/{x}/{y}.png)
- user-agent:    UA header (default piaware-map-tracks/1.0)
- http-timeout:  seconds for HTTP (default 5)
- tile-delay:    ms between tile requests (default 120)
- tile-cache:    directory to cache tiles (optional)
*/

type Config struct {
	// Optional server-wide defaults; zeros mean “use handler defaults”
	Dump1090Base string
	DataPath     string
	TileURL      string
	UserAgent    string
	HTTPTimeout  time.Duration
	TileDelay    time.Duration
	TileCacheDir string
}

type aircraftJSON struct {
	Now      float64     `json:"now"`
	Msgs     int         `json:"messages"`
	Aircraft []airRecord `json:"aircraft"`
}
type airRecord struct {
	Hex     string   `json:"hex"`
	Flight  string   `json:"flight"`
	Lat     *float64 `json:"lat"`
	Lon     *float64 `json:"lon"`
	Seen    *float64 `json:"seen"`
	SeenPos *float64 `json:"seen_pos"`
	Track   *float64 `json:"track"`
	AltBaro *int     `json:"alt_baro"`
	AltGeom *int     `json:"alt_geom"`
}
type point struct {
	lat float64
	lon float64
	t   time.Time
}
type trail struct {
	hex    string
	flight string
	points []point
}

func Handler(cfg Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// ---- parse query params with defaults ----
		q := c.QueryParams()
		get := func(k, def string) string {
			if v := q.Get(k); v != "" {
				return v
			}
			return def
		}
		defDump := choose(cfg.Dump1090Base, "http://localhost/dump1090-fa")
		defData := choose(cfg.DataPath, "/data")
		defTiles := choose(cfg.TileURL, "https://tile.openstreetmap.org/{z}/{x}/{y}.png")
		defUA := choose(cfg.UserAgent, "piaware-map-tracks/1.0 (+local)")
		defHTTP := chooseDur(cfg.HTTPTimeout, 5*time.Second)
		defDelay := chooseDur(cfg.TileDelay, 120*time.Millisecond)

		dumpBase := get("dump1090", defDump)
		dataPath := get("data-path", defData)
		tileURL := get("tile-url", defTiles)
		userAgent := get("user-agent", defUA)
		httpTimeout := parseDurSeconds(get("http-timeout", fmt.Sprintf("%.0f", defHTTP.Seconds())), defHTTP)
		tileDelay := parseDurMillis(get("tile-delay", fmt.Sprintf("%d", defDelay.Milliseconds())), defDelay)
		tileCache := get("tile-cache", cfg.TileCacheDir)

		centerLat := parseFloat(get("center-lat", "0"))
		centerLon := parseFloat(get("center-lon", "0"))
		if centerLat < -85.0511 || centerLat > 85.0511 {
			return echo.NewHTTPError(http.StatusBadRequest, "center-lat out of Web Mercator bounds (-85.0511..85.0511)")
		}
		zoom := clampInt(parseInt(get("zoom", "9")), 2, 19)
		width := clampInt(parseInt(get("width", "1600")), 256, 8000)
		height := clampInt(parseInt(get("height", "1000")), 256, 8000)
		historyN := clampInt(parseInt(get("history", "90")), 0, 119)
		includeLatest := parseBool(get("include-latest", "true"))
		minPoints := clampInt(parseInt(get("min-points", "2")), 1, 1000)
		labelLast := parseBool(get("label-last", "true"))

		theme := strings.ToLower(get("theme", "bw-dither"))
		contrast := parseFloat(get("contrast", "1.35"))
		if contrast <= 0 {
			contrast = 1.0
		}
		gamma := parseFloat(get("gamma", "1.0"))
		invert := parseBool(get("invert", "false"))
		lineWidth := clampInt(parseInt(get("line-width", "0")), 1, 15)
		if lineWidth == 0 {
			if theme == "color" {
				lineWidth = 1
			} else {
				lineWidth = 3
			}
		}

		ctx := c.Request().Context()
		httpClient := &http.Client{Timeout: httpTimeout}

		// ---- 1) Load snapshots ----
		var datasets []*aircraftJSON
		for i := historyN; i >= 0; i-- {
			u := fmt.Sprintf("%s%s/history_%d.json", strings.TrimRight(dumpBase, "/"), dataPath, i)
			aj, err := loadJSON(ctx, httpClient, u, userAgent)
			if err == nil && aj != nil {
				datasets = append(datasets, aj)
			}
		}
		if includeLatest {
			u := fmt.Sprintf("%s%s/aircraft.json", strings.TrimRight(dumpBase, "/"), dataPath)
			aj, err := loadJSON(ctx, httpClient, u, userAgent)
			if err == nil && aj != nil {
				datasets = append(datasets, aj)
			}
		}
		if len(datasets) == 0 {
			return echo.NewHTTPError(http.StatusBadGateway, "no snapshots loaded from dump1090")
		}

		// ---- 2) Build trails chronologically ----
		trails := map[string]*trail{}
		baseTime := time.Now()
		for si, snap := range datasets { // already oldest..newest
			snapTime := baseTime.Add(time.Duration(si-len(datasets)) * time.Second)
			for _, a := range snap.Aircraft {
				if a.Lat == nil || a.Lon == nil {
					continue
				}
				hex := strings.ToUpper(strings.TrimSpace(a.Hex))
				if hex == "" {
					continue
				}
				tr := trails[hex]
				if tr == nil {
					tr = &trail{hex: hex, flight: strings.TrimSpace(a.Flight)}
					trails[hex] = tr
				}
				if f := strings.TrimSpace(a.Flight); f != "" {
					tr.flight = f
				}
				tr.points = append(tr.points, point{lat: *a.Lat, lon: *a.Lon, t: snapTime})
			}
		}
		var list []*trail
		for _, tr := range trails {
			if len(tr.points) >= minPoints {
				list = append(list, tr)
			}
		}
		if len(list) == 0 {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "no drawable tracks; try increasing history or lowering min-points")
		}

		// ---- 3) Render OSM tiles into canvas ----
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		bg := color.RGBA{235, 240, 245, 255}
		draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

		pow2 := 1 << uint(zoom)
		worldSize := float64(256 * pow2)
		centerPx, centerPy := llToPixels(centerLat, centerLon, zoom)
		tlPx := centerPx - float64(width)/2
		tlPy := centerPy - float64(height)/2

		minTileX := int(math.Floor(tlPx / 256.0))
		minTileY := int(math.Floor(tlPy / 256.0))
		maxTileX := int(math.Floor((tlPx+float64(width)-1)/256.0)) + 1
		maxTileY := int(math.Floor((tlPy+float64(height)-1)/256.0)) + 1
		_ = worldSize // reserved if needed later

		// optional simple cache dir
		if tileCache != "" {
			_ = os.MkdirAll(tileCache, 0o755)
		}

		for ty := minTileY; ty < maxTileY; ty++ {
			for tx := minTileX; tx < maxTileX; tx++ {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				txx, tyy := wrapTileXY(tx, ty, zoom)
				tile, err := getTile(ctx, httpClient, tileURL, userAgent, tileCache, zoom, txx, tyy)
				if err != nil {
					// draw a subtle “missing tile” box
					draw.Draw(img, image.Rect(int(float64(tx*256)-tlPx), int(float64(ty*256)-tlPy), int(float64(tx*256)-tlPx)+256, int(float64(ty*256)-tlPy)+256),
						&image.Uniform{color.RGBA{220, 224, 231, 255}}, image.Point{}, draw.Src)
				} else {
					dstX := int(float64(tx*256) - tlPx)
					dstY := int(float64(ty*256) - tlPy)
					draw.Draw(img, image.Rect(dstX, dstY, dstX+256, dstY+256), tile, image.Point{}, draw.Over)
				}
				time.Sleep(tileDelay)
			}
		}

		// ---- 4) Draw trails + labels ----
		project := func(lat, lon float64) (int, int, bool) {
			px, py := llToPixels(lat, lon, zoom)
			x := int(px - tlPx)
			y := int(py - tlPy)
			vis := x >= -1 && x < width+1 && y >= -1 && y < height+1
			return x, y, vis
		}

		for _, tr := range list {
			cclr := colorForHex(tr.hex)
			trackColor := colorForHex(tr.hex)
			if theme != "color" { // e-ink: solid black tracks
				trackColor = color.RGBA{0, 0, 0, 255}
			}
			for i := 1; i < len(tr.points); i++ {
				x0, y0, ok0 := project(tr.points[i-1].lat, tr.points[i-1].lon)
				x1, y1, ok1 := project(tr.points[i].lat, tr.points[i].lon)
				if ok0 || ok1 {
					strokeLine(img, x0, y0, x1, y1, lineWidth, trackColor)
				}
			}
			// last point
			last := tr.points[len(tr.points)-1]
			px, py, _ := project(last.lat, last.lon)
			fillCircle(img, px, py, 4, color.RGBA{0, 0, 0, 255})
			fillCircle(img, px, py, 3, cclr)
			if labelLast {
				lbl := strings.TrimSpace(tr.flight)
				if lbl == "" {
					lbl = tr.hex
				} else {
					lbl = fmt.Sprintf("%s (%s)", lbl, tr.hex)
				}
				drawLabel(img, px+6, py-6, lbl, color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 210})
			}
		}

		// ---- 5) Title, legend from latest snapshot, attribution ----
		title := fmt.Sprintf("PiAware Tracks  |  center=(%.4f, %.4f)  zoom=%d  history=%d",
			centerLat, centerLon, zoom, historyN)
		drawLabel(img, 12, 20, title, color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 210})

		latest := datasets[len(datasets)-1]
		type legendRow struct {
			key   string
			color color.RGBA
		}
		var rows []legendRow
		seen := map[string]bool{}
		for _, a := range latest.Aircraft {
			hex := strings.ToUpper(strings.TrimSpace(a.Hex))
			if hex == "" || seen[hex] {
				continue
			}
			seen[hex] = true
			lbl := strings.TrimSpace(a.Flight)
			if lbl == "" {
				lbl = hex
			} else {
				lbl = fmt.Sprintf("%s (%s)", lbl, hex)
			}
			rows = append(rows, legendRow{key: lbl, color: colorForHex(hex)})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].key < rows[j].key })

		legendX, legendY := 12, 40
		for _, r := range rows {
			fillRect(img, legendX, legendY-10, 18, 6, r.color)
			drawLabel(img, legendX+12, legendY, r.key, color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 200})
			legendY += 16
			if legendY > height-30 {
				break
			}
		}

		attr := "© OpenStreetMap contributors"
		w := textWidth(attr)
		drawLabel(img, width-w-12, height-10, attr, color.RGBA{30, 30, 30, 255}, color.RGBA{255, 255, 255, 210})

		// ---- 6) Write PNG to response ----
		var outImg image.Image = img
		switch theme {
		case "color":
			// no post-processing
		case "gray":
			outImg = toGray(img)
			outImg = adjustGray(outImg.(*image.Gray), contrast, gamma, invert)
		case "gray4":
			g := toGray(img)
			g = adjustGray(g, contrast, gamma, invert)
			outImg = quantizeGrayLevels(g, []uint8{0, 85, 170, 255}) // 4-level
		case "bw":
			g := toGray(img)
			g = adjustGray(g, contrast, gamma, invert)
			outImg = thresholdBW(g, 128) // hard threshold
		case "bw-dither":
			g := toGray(img)
			g = adjustGray(g, contrast, gamma, invert)
			outImg = floydSteinbergBW(g) // error-diffused 1-bit
		default:
			g := toGray(img)
			g = adjustGray(g, contrast, gamma, invert)
			outImg = floydSteinbergBW(g)
		}

		// Encode outImg instead of img:
		var buf bytes.Buffer
		if err := png.Encode(&buf, outImg); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "encode png failed")
		}
		c.Response().Header().Set("Cache-Control", "no-cache")
		len := buf.Len()
		c.Response().Header().Set("Content-Length", strconv.Itoa(len))
		return c.Blob(http.StatusOK, "image/png", buf.Bytes())
	}
}

// ---------- helpers ----------

func choose(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
func chooseDur(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}
func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "1", "t", "true", "y", "yes":
		return true
	}
	return false
}
func parseDurSeconds(s string, def time.Duration) time.Duration {
	if v, err := strconv.Atoi(s); err == nil && v >= 0 {
		return time.Duration(v) * time.Second
	}
	return def
}
func parseDurMillis(s string, def time.Duration) time.Duration {
	if v, err := strconv.Atoi(s); err == nil && v >= 0 {
		return time.Duration(v) * time.Millisecond
	}
	return def
}
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
