package main

import (
	"errors"
	"flag"
	"net/url"

	"github.com/callumj/adsb-apis/pkg/adsbdb"
	"github.com/callumj/adsb-apis/pkg/config"
	"github.com/callumj/adsb-apis/pkg/dump1090"
	"github.com/callumj/adsb-apis/pkg/handlers"
	"github.com/callumj/adsb-apis/pkg/push"
	"github.com/callumj/adsb-apis/pkg/tracks"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	configFile := flag.String("config-file", "", "Config file path (YAML)")
	flag.Parse()

	if *configFile == "" {
		panic(errors.New("config file must be specified"))
	}

	conf, err := config.LoadConfig(*configFile)
	if err != nil {
		panic(err)
	}

	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	d := dump1090.NewDump1090(conf.AircraftJsonUrl)
	a := adsbdb.NewAdsbdb()
	h := &handlers.Handlers{Config: conf, Dump1090: d, AdsbDB: a}

	// Routes
	e.GET("/nearby", h.GetNearby)
	e.GET("/overhead", h.GetOverhead)

	parsedUrl, _ := url.Parse(conf.AircraftJsonUrl)
	baseUrl := parsedUrl.Scheme + "://" + parsedUrl.Host
	e.GET("/tracks.png", tracks.Handler(tracks.Config{
		// Optional defaults (can be overridden per request by query params)
		Dump1090Base: baseUrl,
		TileCacheDir: "./tile_cache",
	}))

	// Push
	if conf.PushWebhookUrl != "" {
		p := push.NewPush(conf, d, a)
		p.Start()
		defer p.Stop()
	}

	// Start server
	e.Logger.Fatal(e.Start(conf.HttpListenAddr))
}
