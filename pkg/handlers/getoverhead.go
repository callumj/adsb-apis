package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type OverheadResponse struct {
	Flight     *AircraftDetail `json:"flight"`
	SimpleText string          `json:"simple_text"` // For simple text display
}

func (h *Handlers) GetOverhead(c echo.Context) error {
	da, err := h.Dump1090.GetAircraft()
	if err != nil {
		c.Error(err)
	}

	o := da.GetNearby(h.Config.Latitude, h.Config.Longitude, 0.1)

	resp := &OverheadResponse{}

	if len(o) != 0 && o[0].Flight != "" {
		resp.Flight = h.aircraft2Detail(o[0])
		resp.SimpleText = resp.Flight.SimpleText
	}

	_ = c.JSON(http.StatusOK, resp)
	return nil
}
