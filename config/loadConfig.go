package config

import (
	"arbitraj-bot/core"
	"encoding/json"
	"os"
)

func LoadConfig(path string) (core.Config, error) {
	var config core.Config
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer file.Close()
	err = json.NewDecoder(file).Decode(&config)
	return config, err
}
