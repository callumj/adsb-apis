package handlers

import (
	"net/http"
	"strings"

	"github.com/callumj/adsb-apis/pkg/adsbdb"
	"github.com/callumj/adsb-apis/pkg/dump1090"
	"github.com/labstack/echo/v4"
)

type AircraftDetail struct {
	Flight                 string `json:"flight"`
	DestinationAirportName string `json:"destination_airport_name"`
	DestinationMuni        string `json:"destination_muni"`
	OriginAirportName      string `json:"origin_airport_name"`
	OriginMuni             string `json:"origin_airport_muni"`
	AircraftType           string `json:"aircraft_type"`
	Airline                string `json:"airline"`
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

		f := &AircraftDetail{Flight: strings.TrimSpace(a.Flight)}
		r.Flights = append(r.Flights, f)
		d, err := adsbdb.GetCallsign(a.Flight)
		if err != nil {
			continue
		}

		f.Airline = d.Response.Flightroute.Airline.Name
		f.DestinationAirportName = d.Response.Flightroute.Destination.Name
		f.DestinationMuni = d.Response.Flightroute.Destination.Municipality
		f.OriginAirportName = d.Response.Flightroute.Origin.Name
		f.OriginMuni = d.Response.Flightroute.Origin.Municipality

		r, err := adsbdb.GetRegistration(a.Hex)
		if err != nil {
			continue
		}
		f.AircraftType = r.Response.Aircraft.IcaoType
	}

	c.JSON(http.StatusOK, r)
	return nil
}
