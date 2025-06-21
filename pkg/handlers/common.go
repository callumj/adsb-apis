package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/callumj/adsb-apis/pkg/dump1090"
	"github.com/labstack/echo/v4"
)

type AircraftDetail struct {
	Flight                 string  `json:"flight"`
	DestinationAirportName string  `json:"destination_airport_name"`
	DestinationMuni        string  `json:"destination_muni"`
	OriginAirportName      string  `json:"origin_airport_name"`
	OriginMuni             string  `json:"origin_airport_muni"`
	AircraftType           string  `json:"aircraft_type"`
	Airline                string  `json:"airline"`
	SimpleText             string  `json:"simple_text,omitempty"` // For simple text display
	DistMiles              float64 `json:"dist_miles,omitempty"`  // Distance from the observer, if calculated
}

type NearbyResponse struct {
	Flights []*AircraftDetail `json:"flights"`
}

func (h *Handlers) renderResults(o []*dump1090.Aircraft, c echo.Context) {
	r := &NearbyResponse{}
	r.Flights = []*AircraftDetail{}

	for _, a := range o {
		if a.Flight == "" {
			continue
		}

		detail := h.aircraft2Detail(a)
		if detail == nil {
			continue
		}
		r.Flights = append(r.Flights, detail)
	}

	_ = c.JSON(http.StatusOK, r)
}

func (h *Handlers) aircraft2Detail(a *dump1090.Aircraft) *AircraftDetail {
	f := &AircraftDetail{Flight: strings.TrimSpace(a.Flight)}
	d, err := h.AdsbDB.GetCallsign(a.Flight)
	if err != nil {
		return nil
	}

	f.Airline = d.Response.Flightroute.Airline.Name
	f.DestinationAirportName = d.Response.Flightroute.Destination.Name
	f.DestinationMuni = d.Response.Flightroute.Destination.Municipality
	f.OriginAirportName = d.Response.Flightroute.Origin.Name
	f.OriginMuni = d.Response.Flightroute.Origin.Municipality
	f.DistMiles = a.DistMiles

	f.SimpleText = fmt.Sprintf("%s %s -> %s", f.Flight, f.OriginMuni, f.DestinationMuni)

	reg, err := h.AdsbDB.GetRegistration(a.Hex)
	if err != nil {
		return nil
	}
	f.AircraftType = reg.Response.Aircraft.IcaoType

	return f
}
