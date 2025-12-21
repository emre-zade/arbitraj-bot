package config

import (
	"arbitraj-bot/core"
	"encoding/json"
	"os"
)

// SaveConfig, güncel konfigürasyon yapısını belirtilen yola JSON olarak kaydeder.
func SaveConfig(path string, config core.Config) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(config)
}
