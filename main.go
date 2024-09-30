package main

import (
	"errors"
	"flag"

	"github.com/callumj/adsb-apis/pkg/config"
	"github.com/callumj/adsb-apis/pkg/handlers"
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

	h := &handlers.Handlers{Config: conf}

	// Routes
	e.GET("/nearby", h.GetNearby)

	// Start server
	e.Logger.Fatal(e.Start(conf.HttpListenAddr))
}
