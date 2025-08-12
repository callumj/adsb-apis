package adsbdb

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

func (a *Adsbdb) Save(ns, key string, value interface{}) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	if value == nil {
		return errors.New("value cannot be nil")
	}

	// Serialize the value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// write the data to the cache file
	cacheFile := a.cacheFilename(ns, key)

	_ = os.MkdirAll(filepath.Dir(cacheFile), 0755) // Ensure the directory exists)

	err = os.WriteFile(cacheFile, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

func (a *Adsbdb) Load(ns, key string, cacheDuration time.Duration, value interface{}) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	cacheFile := a.cacheFilename(ns, key)

	// check last modified time
	if cacheDuration != 0 {
		info, err := os.Stat(cacheFile)
		if err == nil && info.ModTime().Add(cacheDuration).After(time.Now()) {
			return errors.New("expired cache")
		}
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, value)
	if err != nil {
		return err
	}

	return nil
}

func (a *Adsbdb) cacheFilename(ns, key string) string {
	if ns == "" {
		ns = "default"
	}
	return a.cacheDir + "/" + ns + "/" + key + ".json"
}
