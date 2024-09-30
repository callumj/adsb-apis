package dump1090

import (
	"encoding/json"
	"net/http"

	"github.com/jftuga/geodist"
)

type Aircraft struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	Flight    string  `json:"flight"`
}

type Dump1090Response struct {
	Aircraft []*Aircraft
}

func (d *Dump1090Response) GetNearby(lat, lon, dist float64) []*Aircraft {
	list := []*Aircraft{}

	loc := geodist.Coord{Lat: lat, Lon: lon}
	for _, a := range d.Aircraft {
		pos := geodist.Coord{Lat: a.Latitude, Lon: a.Longitude}

		miles, _ := geodist.HaversineDistance(loc, pos)

		if miles < dist {
			list = append(list, a)
		}
	}

	return list
}

type Dump1090 struct {
	url string
}

func NewDump1090(url string) *Dump1090 {
	return &Dump1090{
		url: url,
	}
}

func (d *Dump1090) GetAircraft() (*Dump1090Response, error) {
	res, err := http.Get(d.url)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	dec := json.NewDecoder(res.Body)

	du := &Dump1090Response{}
	err = dec.Decode(du)
	return du, err
}
