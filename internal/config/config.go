package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all codex-dock configuration.
type Config struct {
	DefaultImage    string `mapstructure:"default_image"`
	DefaultTokenTTL int    `mapstructure:"default_token_ttl"`
	NetworkName     string `mapstructure:"network_name"`
	Verbose         bool   `mapstructure:"verbose"`
	Debug           bool   `mapstructure:"debug"`
}

// Load reads the configuration from the standard path and environment.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultConfig(), nil
	}

	viper.AddConfigPath(filepath.Join(home, ".config", "codex-dock"))
	viper.SetConfigName("config")
	viper.SetConfigType("toml")

	// Defaults
	viper.SetDefault("default_image", "codex-dock:latest")
	viper.SetDefault("default_token_ttl", 3600)
	viper.SetDefault("network_name", "dock-net")

	viper.AutomaticEnv()

	_ = viper.ReadInConfig() // ignore missing config file

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return defaultConfig(), nil
	}
	return &cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		DefaultImage:    "codex-dock:latest",
		DefaultTokenTTL: 3600,
		NetworkName:     "dock-net",
	}
}
