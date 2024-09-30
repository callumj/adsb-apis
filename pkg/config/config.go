package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HttpListenAddr  string  `yaml:"http_listen_addr"`
	AircraftJsonUrl string  `yaml:"aircraft_json_url"`
	Latitude        float64 `yaml:"latitude"`
	Longitude       float64 `yaml:"longitude"`
	MaxDistance     float64 `yaml:"max_distance"`
}

func LoadConfig(path string) (*Config, error) {
	filename, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config *Config

	err = yaml.Unmarshal(yamlFile, &config)
	return config, err
}
