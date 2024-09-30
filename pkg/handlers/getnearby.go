package handlers

import (
	"net/http"

	"github.com/callumj/adsb-apis/pkg/adsbdb"
	"github.com/callumj/adsb-apis/pkg/dump1090"
	"github.com/labstack/echo/v4"
)

type AircraftDetail struct {
	Flight                 string `json:"flight"`
	DestinationAirportName string `json:"destination_airport_name"`
}

type NearbyResponse struct {
	Flights []*AircraftDetail `json:"flights"`
}

func (h *Handlers) GetNearby(c echo.Context) error {
	i := dump1090.NewDump1090(h.Config.AircraftJsonUrl)
	da, err := i.GetAircraft()
	if err != nil {
		c.Error(err)
	}

	o := da.GetNearby(h.Config.Latitude, h.Config.Longitude, h.Config.MaxDistance)

	r := &NearbyResponse{}
	r.Flights = []*AircraftDetail{}

	for _, a := range o {
		if a.Flight == "" {
			continue
		}

		f := &AircraftDetail{Flight: a.Flight}
		r.Flights = append(r.Flights, f)
		d, err := adsbdb.Get(a.Flight)
		if err != nil {
			continue
		}

		f.DestinationAirportName = d.Response.Flightroute.Destination.Name
	}

	c.JSON(http.StatusOK, r)
	return nil
}
