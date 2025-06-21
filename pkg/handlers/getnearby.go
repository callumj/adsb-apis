package handlers

import (
	"github.com/labstack/echo/v4"
)

func (h *Handlers) GetNearby(c echo.Context) error {
	da, err := h.Dump1090.GetAircraft()
	if err != nil {
		c.Error(err)
	}

	o := da.GetNearby(h.Config.Latitude, h.Config.Longitude, h.Config.MaxDistance)

	h.renderResults(o, c)
	return nil
}
