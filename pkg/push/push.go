package push

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/callumj/adsb-apis/pkg/adsbdb"
	"github.com/callumj/adsb-apis/pkg/config"
	"github.com/callumj/adsb-apis/pkg/dump1090"
	"github.com/rs/zerolog/log"
)

type Push struct {
	Config   *config.Config
	Dump1090 *dump1090.Dump1090
	AdsbDB   *adsbdb.Adsbdb
	Ticker   time.Ticker
}

type PushData struct {
	FlightCode string `json:"flight_code"`
	Route      string `json:"route"`
}

func NewPush(c *config.Config, d *dump1090.Dump1090, a *adsbdb.Adsbdb) *Push {
	return &Push{
		Config:   c,
		Dump1090: d,
		AdsbDB:   a,
	}
}

func (p *Push) Start() {
	p.pushData()
	p.Ticker = *time.NewTicker(time.Duration(p.Config.PushIntervalSec) * time.Second)
	go func() {
		for {
			select {
			case <-p.Ticker.C:
				p.pushData()
			}
		}
	}()
}

func (p *Push) Stop() {
	p.Ticker.Stop()
}

func (p *Push) pushData() {
	da, err := p.Dump1090.GetAircraft()
	if err != nil {
		// Handle error (e.g., log it)
		return
	}

	o := da.GetNearby(p.Config.Latitude, p.Config.Longitude, p.Config.MaxDistance)

	d := &PushData{}

	for _, a := range o {
		if a.Flight == "" {
			continue
		}

		detail, err := p.AdsbDB.GetCallsign(a.Flight)
		if err != nil {
			log.Error().Err(err).Str("flight", a.Flight).Msg("Failed to get callsign details")
			continue
		}

		d.FlightCode = a.Flight
		d.Route = fmt.Sprintf("%s (%s) -> %s (%s)",
			detail.Response.Flightroute.Origin.Municipality,
			detail.Response.Flightroute.Origin.IataCode,
			detail.Response.Flightroute.Destination.Municipality,
			detail.Response.Flightroute.Destination.IataCode,
		)
	}

	err = p.SendPushData(d)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send push data")
	} else {
		log.Info().Str("flight_code", d.FlightCode).Msg("Push data sent successfully")
	}
}

func (p *Push) SendPushData(data *PushData) error {
	payload := map[string]interface{}{
		"merge_variables": map[string]string{
			"flight_code": data.FlightCode,
			"route":       data.Route,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal PushData: %w", err)
	}

	req, err := http.NewRequest("POST", p.Config.PushWebhookUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK HTTP status: %s", resp.Status)
	}

	return nil
}
