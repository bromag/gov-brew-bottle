package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	NexusBaseURL string
	NexusUser    string
	NexusPass    string
	NexusPrefix  string

	DefaultTag     string
	DefaultWorkdir string

	BrewBin        string
	HomebrewPrefix string
	TapWorkdir     string
}

func LoadEnv() {
	_ = godotenv.Load(".env")
}

func FromEnv() Config {
	return Config{
		NexusBaseURL:   os.Getenv("NEXUS_BASE_URL"),
		NexusUser:      os.Getenv("NEXUS_USER"),
		NexusPass:      os.Getenv("NEXUS_PASS"),
		NexusPrefix:    getenvDefault("NEXUS_PREFIX", "bottles"),
		DefaultTag:     getenvDefault("DEFAULT_TAG", ""),
		DefaultWorkdir: getenvDefault("DEFAULT_WORKDIR", "./dist"),
		BrewBin:        getenvDefault("BREW_BIN", "brew"),
		HomebrewPrefix: getenvDefault("HOMEBREW_PREFIX", "/opt/homebrew"),
		TapWorkdir:     getenvDefault("TAP_WORKDIR", "./tap"),
	}
}

func (c Config) ValidateForUpload() error {
	if c.NexusBaseURL == "" {
		return fmt.Errorf("NEXUS_BASE_URL is missing")
	}
	if c.NexusUser == "" || c.NexusPass == "" {
		return fmt.Errorf("NEXUS_USER or NEXUS_PASS is missing")
	}
	if c.NexusPrefix == "" {
		return fmt.Errorf("NEXUS_PREFIX is missing")
	}
	return nil
}

func getenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
