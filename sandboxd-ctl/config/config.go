package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"sandboxd-o/pkg/envutil"
)

const DefaultConfigPath = "/var/lib/sandboxd/sbxctl_config.json"

type Config struct {
	Server  string        `json:"server,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty"`
	Output  string        `json:"output,omitempty"`
	Limit   int           `json:"limit,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Server:  "http://127.0.0.1:8082",
		Timeout: 10 * time.Second,
		Limit:   100,
	}
}

func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	path = strings.TrimSpace(path)
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return Config{}, fmt.Errorf("read config file %q: %w", path, err)
			}
		} else {
			var fileCfg struct {
				Server  *string `json:"server,omitempty"`
				Timeout *string `json:"timeout,omitempty"`
				Output  *string `json:"output,omitempty"`
				Limit   *int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(raw, &fileCfg); err != nil {
				return Config{}, fmt.Errorf("parse config file %q: %w", path, err)
			}
			if fileCfg.Server != nil {
				cfg.Server = *fileCfg.Server
			}
			if fileCfg.Timeout != nil {
				if v, err := time.ParseDuration(*fileCfg.Timeout); err == nil {
					cfg.Timeout = v
				}
			}
			if fileCfg.Output != nil {
				cfg.Output = *fileCfg.Output
			}
			if fileCfg.Limit != nil {
				cfg.Limit = *fileCfg.Limit
			}
		}
	}

	cfg.Server = envutil.Get("SBXCTL_SERVER", cfg.Server)
	if strings.TrimSpace(cfg.Server) == "" {
		cfg.Server = "http://127.0.0.1:8082"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Limit <= 0 {
		cfg.Limit = 100
	}

	return cfg, nil
}
