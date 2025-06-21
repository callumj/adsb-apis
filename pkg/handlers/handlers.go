package handlers

import (
	"github.com/callumj/adsb-apis/pkg/adsbdb"
	"github.com/callumj/adsb-apis/pkg/config"
	"github.com/callumj/adsb-apis/pkg/dump1090"
)

type Handlers struct {
	Config   *config.Config
	Dump1090 *dump1090.Dump1090
	AdsbDB   *adsbdb.Adsbdb
}
