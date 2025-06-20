package handlers

import (
	"github.com/callumj/adsb-apis/pkg/dump1090"
	"github.com/labstack/echo/v4"
)

func (h *Handlers) GetOverhead(c echo.Context) error {
	i := dump1090.NewDump1090(h.Config.AircraftJsonUrl)
	da, err := i.GetAircraft()
	if err != nil {
		c.Error(err)
	}

	o := da.GetNearby(h.Config.Latitude, h.Config.Longitude, 2)

	renderResults(o, c)
	return nil
}
