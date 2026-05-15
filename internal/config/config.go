package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	APIURL       string `json:"apiUrl"`
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

func Resolve(apiURLFlag, tokenFlag string) (Config, error) {
	cfg := Config{}
	fileCfg, _ := Load()

	cfg.APIURL = strings.TrimSpace(fileCfg.APIURL)
	cfg.Token = strings.TrimSpace(fileCfg.Token)
	cfg.RefreshToken = strings.TrimSpace(fileCfg.RefreshToken)

	if envAPI := strings.TrimSpace(os.Getenv("EINAR_API_URL")); envAPI != "" {
		cfg.APIURL = envAPI
	}
	if envToken := strings.TrimSpace(os.Getenv("EINAR_TOKEN")); envToken != "" {
		cfg.Token = envToken
	}
	if envRefresh := strings.TrimSpace(os.Getenv("EINAR_REFRESH_TOKEN")); envRefresh != "" {
		cfg.RefreshToken = envRefresh
	}

	if strings.TrimSpace(apiURLFlag) != "" {
		cfg.APIURL = strings.TrimSpace(apiURLFlag)
	}
	if strings.TrimSpace(tokenFlag) != "" {
		cfg.Token = strings.TrimSpace(tokenFlag)
	}

	if cfg.APIURL == "" {
		return cfg, errors.New("falta EINAR_API_URL (flag --api-url, env EINAR_API_URL o config local)")
	}

	u, err := url.Parse(cfg.APIURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return cfg, fmt.Errorf("api url inválida: %q", cfg.APIURL)
	}

	cfg.APIURL = strings.TrimRight(cfg.APIURL, "/")
	return cfg, nil
}

func ConfigPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, ".einar", "config.json"), nil
}

func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return nil
}

func MaskToken(token string) string {
	t := strings.TrimSpace(token)
	if len(t) <= 8 {
		return "***"
	}
	return t[:7] + "..." + t[len(t)-4:]
}
