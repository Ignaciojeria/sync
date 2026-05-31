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
	APIURL             string `json:"apiUrl,omitempty"`
	Token              string `json:"token,omitempty"`
	RefreshToken       string `json:"refreshToken,omitempty"`
	LastProjectID      string
	LastProjectSlug    string
	MutagenDestination string
	MutagenSessionName string
	LastVMName         string
	LastVMHTTPSURL     string
	LastVMSshDest      string
	ProjectAPIToken    string
	ProjectDBName      string
	ProjectDBUser      string
	ProjectDBPassword  string
	ProjectDBHost      string
	ProjectDBPort      int
	ProjectDatabaseURL string
	WorkspaceBranch    string
}

type projectDiskConfig struct {
	Project struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"project,omitempty"`
	Database struct {
		URL string `json:"url,omitempty"`
	} `json:"database,omitempty"`
}

func Resolve(apiURLFlag, tokenFlag string) (Config, error) {
	cfg := Config{}
	globalCfg, _ := LoadGlobal()
	projectCfg, _ := Load()

	// Credenciales/API desde config global del CLI
	cfg.APIURL = strings.TrimSpace(globalCfg.APIURL)
	cfg.Token = strings.TrimSpace(globalCfg.Token)
	cfg.RefreshToken = strings.TrimSpace(globalCfg.RefreshToken)

	// Estado de workspace/proyecto desde config local del repo
	cfg.LastProjectID = strings.TrimSpace(projectCfg.LastProjectID)
	cfg.LastProjectSlug = strings.TrimSpace(projectCfg.LastProjectSlug)
	cfg.MutagenDestination = strings.TrimSpace(projectCfg.MutagenDestination)
	cfg.MutagenSessionName = strings.TrimSpace(projectCfg.MutagenSessionName)
	cfg.LastVMName = strings.TrimSpace(projectCfg.LastVMName)
	cfg.LastVMHTTPSURL = strings.TrimSpace(projectCfg.LastVMHTTPSURL)
	cfg.LastVMSshDest = strings.TrimSpace(projectCfg.LastVMSshDest)
	cfg.ProjectAPIToken = strings.TrimSpace(projectCfg.ProjectAPIToken)
	cfg.ProjectDBName = strings.TrimSpace(projectCfg.ProjectDBName)
	cfg.ProjectDBUser = strings.TrimSpace(projectCfg.ProjectDBUser)
	cfg.ProjectDBPassword = strings.TrimSpace(projectCfg.ProjectDBPassword)
	cfg.ProjectDBHost = strings.TrimSpace(projectCfg.ProjectDBHost)
	cfg.ProjectDBPort = projectCfg.ProjectDBPort
	cfg.ProjectDatabaseURL = strings.TrimSpace(projectCfg.ProjectDatabaseURL)
	cfg.WorkspaceBranch = strings.TrimSpace(projectCfg.WorkspaceBranch)

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
		return cfg, errors.New("falta EINAR_API_URL (flag --api-url, env EINAR_API_URL o config global del CLI)")
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

func GlobalConfigPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	execDir := filepath.Dir(execPath)
	return filepath.Join(execDir, ".einar", "config.json"), nil
}

func homeGlobalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".einar", "cli-config.json"), nil
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

	var disk projectDiskConfig
	if err := json.Unmarshal(b, &disk); err != nil {
		return Config{}, err
	}

	return Config{
		LastProjectID:      strings.TrimSpace(disk.Project.ID),
		LastProjectSlug:    strings.TrimSpace(disk.Project.Name),
		ProjectDatabaseURL: strings.TrimSpace(disk.Database.URL),
	}, nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	var onDisk projectDiskConfig
	onDisk.Project.ID = strings.TrimSpace(cfg.LastProjectID)
	onDisk.Project.Name = strings.TrimSpace(cfg.LastProjectSlug)
	onDisk.Database.URL = strings.TrimSpace(cfg.ProjectDatabaseURL)

	b, err := json.MarshalIndent(onDisk, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return nil
}

func LoadGlobal() (Config, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(path)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return Config{}, err
	}

	// Compatibilidad: fallback a config global en HOME
	homePath, herr := homeGlobalConfigPath()
	if herr != nil {
		return Config{}, err
	}
	hb, herr := os.ReadFile(homePath)
	if herr != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(hb, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func SaveGlobal(cfg Config) error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	onDisk := Config{
		APIURL:       strings.TrimSpace(cfg.APIURL),
		Token:        strings.TrimSpace(cfg.Token),
		RefreshToken: strings.TrimSpace(cfg.RefreshToken),
	}
	b, err := json.MarshalIndent(onDisk, "", "  ")
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
