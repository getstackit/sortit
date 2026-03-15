package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type cliConfig struct {
	APIURL string `json:"apiUrl"`
	Token  string `json:"token"`
}

func defaultConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ".splat-cli.json"
	}
	return filepath.Join(configDir, "splat", "config.json")
}

func loadCLIConfig(path string) (cliConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cliConfig{}, nil
		}
		return cliConfig{}, fmt.Errorf("read cli config: %w", err)
	}

	var cfg cliConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cliConfig{}, fmt.Errorf("decode cli config: %w", err)
	}
	cfg.APIURL = strings.TrimSpace(cfg.APIURL)
	cfg.Token = strings.TrimSpace(cfg.Token)
	return cfg, nil
}

func saveCLIConfig(path string, cfg cliConfig) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultConfigPath()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create cli config dir: %w", err)
	}

	cfg.APIURL = strings.TrimSpace(cfg.APIURL)
	cfg.Token = strings.TrimSpace(cfg.Token)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cli config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write cli config: %w", err)
	}
	return nil
}
