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
	LastProjectID      string `json:"lastProjectId,omitempty"`
	LastProjectSlug    string `json:"lastProjectSlug,omitempty"`
	MutagenDestination string `json:"mutagenDestination,omitempty"`
	MutagenSessionName string `json:"mutagenSessionName,omitempty"`
	LastVMName         string `json:"lastVmName,omitempty"`
	LastVMHTTPSURL     string `json:"lastVmHttpsUrl,omitempty"`
	LastVMSshDest      string `json:"lastVmSshDest,omitempty"`
	ProjectAPIToken    string `json:"projectApiToken,omitempty"`
	ProjectDBName      string `json:"projectDbName,omitempty"`
	ProjectDBUser      string `json:"projectDbUser,omitempty"`
	ProjectDBPassword  string `json:"projectDbPassword,omitempty"`
	ProjectDBHost      string `json:"projectDbHost,omitempty"`
	ProjectDBPort      int    `json:"projectDbPort,omitempty"`
	ProjectDatabaseURL string `json:"projectDatabaseUrl,omitempty"`
	WorkspaceBranch    string `json:"workspaceBranch,omitempty"`
	OIDCIssuer         string `json:"oidcIssuer,omitempty"`
	OIDCClientID       string `json:"oidcClientId,omitempty"`
	OIDCClientSecret   string `json:"oidcClientSecret,omitempty"`
	CasdoorOrg         string `json:"casdoorOrg,omitempty"`
	CasdoorApplication string `json:"casdoorApplication,omitempty"`
}

type projectDiskConfig struct {
	Project struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"project,omitempty"`
	Database struct {
		URL string `json:"url,omitempty"`
	} `json:"database,omitempty"`
	Workspace struct {
		Branch string `json:"branch,omitempty"`
	} `json:"workspace,omitempty"`
	Sync struct {
		Destination string `json:"destination,omitempty"`
		SessionName string `json:"sessionName,omitempty"`
	} `json:"sync,omitempty"`
	VM struct {
		Name           string `json:"name,omitempty"`
		HTTPSURL       string `json:"httpsUrl,omitempty"`
		SSHDestination string `json:"sshDestination,omitempty"`
	} `json:"vm,omitempty"`
	DatabaseRuntime struct {
		Name     string `json:"name,omitempty"`
		User     string `json:"user,omitempty"`
		Password string `json:"password,omitempty"`
		Host     string `json:"host,omitempty"`
		Port     int    `json:"port,omitempty"`
	} `json:"databaseRuntime,omitempty"`
	Auth struct {
		Issuer       string `json:"issuer,omitempty"`
		ClientID     string `json:"clientId,omitempty"`
		ClientSecret string `json:"clientSecret,omitempty"`
		Organization string `json:"organization,omitempty"`
		Application  string `json:"application,omitempty"`
	} `json:"auth,omitempty"`
	Identity struct {
		Issuer          string `json:"issuer,omitempty"`
		ClientID        string `json:"clientId,omitempty"`
		ClientSecret    string `json:"clientSecret,omitempty"`
		ClientSecretRef string `json:"clientSecretRef,omitempty"`
		Organization    string `json:"organization,omitempty"`
		Application     string `json:"application,omitempty"`
	} `json:"identity,omitempty"`

	// Compatibilidad con formatos legacy flat.
	LastProjectID      string `json:"lastProjectId,omitempty"`
	LastProjectSlug    string `json:"lastProjectSlug,omitempty"`
	MutagenDestination string `json:"mutagenDestination,omitempty"`
	MutagenSessionName string `json:"mutagenSessionName,omitempty"`
	LastVMName         string `json:"lastVmName,omitempty"`
	LastVMHTTPSURL     string `json:"lastVmHttpsUrl,omitempty"`
	LastVMSshDest      string `json:"lastVmSshDest,omitempty"`
	ProjectAPIToken    string `json:"projectApiToken,omitempty"`
	ProjectDBName      string `json:"projectDbName,omitempty"`
	ProjectDBUser      string `json:"projectDbUser,omitempty"`
	ProjectDBPassword  string `json:"projectDbPassword,omitempty"`
	ProjectDBHost      string `json:"projectDbHost,omitempty"`
	ProjectDBPort      int    `json:"projectDbPort,omitempty"`
	ProjectDatabaseURL string `json:"projectDatabaseUrl,omitempty"`
	WorkspaceBranch    string `json:"workspaceBranch,omitempty"`
	OIDCIssuer         string `json:"oidcIssuer,omitempty"`
	OIDCClientID       string `json:"oidcClientId,omitempty"`
	OIDCClientSecret   string `json:"oidcClientSecret,omitempty"`
	CasdoorOrg         string `json:"casdoorOrg,omitempty"`
	CasdoorApplication string `json:"casdoorApplication,omitempty"`
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

	projectID := strings.TrimSpace(disk.Project.ID)
	if projectID == "" {
		projectID = strings.TrimSpace(disk.LastProjectID)
	}
	projectSlug := strings.TrimSpace(disk.Project.Name)
	if projectSlug == "" {
		projectSlug = strings.TrimSpace(disk.LastProjectSlug)
	}
	databaseURL := strings.TrimSpace(disk.Database.URL)
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(disk.ProjectDatabaseURL)
	}
	workspaceBranch := strings.TrimSpace(disk.Workspace.Branch)
	if workspaceBranch == "" {
		workspaceBranch = strings.TrimSpace(disk.WorkspaceBranch)
	}
	mutagenDestination := strings.TrimSpace(disk.Sync.Destination)
	if mutagenDestination == "" {
		mutagenDestination = strings.TrimSpace(disk.MutagenDestination)
	}
	mutagenSessionName := strings.TrimSpace(disk.Sync.SessionName)
	if mutagenSessionName == "" {
		mutagenSessionName = strings.TrimSpace(disk.MutagenSessionName)
	}
	lastVMName := strings.TrimSpace(disk.VM.Name)
	if lastVMName == "" {
		lastVMName = strings.TrimSpace(disk.LastVMName)
	}
	lastVMHTTPSURL := strings.TrimSpace(disk.VM.HTTPSURL)
	if lastVMHTTPSURL == "" {
		lastVMHTTPSURL = strings.TrimSpace(disk.LastVMHTTPSURL)
	}
	lastVMSshDest := strings.TrimSpace(disk.VM.SSHDestination)
	if lastVMSshDest == "" {
		lastVMSshDest = strings.TrimSpace(disk.LastVMSshDest)
	}
	projectDBName := strings.TrimSpace(disk.DatabaseRuntime.Name)
	if projectDBName == "" {
		projectDBName = strings.TrimSpace(disk.ProjectDBName)
	}
	projectDBUser := strings.TrimSpace(disk.DatabaseRuntime.User)
	if projectDBUser == "" {
		projectDBUser = strings.TrimSpace(disk.ProjectDBUser)
	}
	projectDBPassword := strings.TrimSpace(disk.DatabaseRuntime.Password)
	if projectDBPassword == "" {
		projectDBPassword = strings.TrimSpace(disk.ProjectDBPassword)
	}
	projectDBHost := strings.TrimSpace(disk.DatabaseRuntime.Host)
	if projectDBHost == "" {
		projectDBHost = strings.TrimSpace(disk.ProjectDBHost)
	}
	projectDBPort := disk.DatabaseRuntime.Port
	if projectDBPort == 0 {
		projectDBPort = disk.ProjectDBPort
	}
	oidcIssuer := strings.TrimSpace(disk.Identity.Issuer)
	if oidcIssuer == "" {
		oidcIssuer = strings.TrimSpace(disk.Auth.Issuer)
	}
	if oidcIssuer == "" {
		oidcIssuer = strings.TrimSpace(disk.OIDCIssuer)
	}
	oidcClientID := strings.TrimSpace(disk.Identity.ClientID)
	if oidcClientID == "" {
		oidcClientID = strings.TrimSpace(disk.Auth.ClientID)
	}
	if oidcClientID == "" {
		oidcClientID = strings.TrimSpace(disk.OIDCClientID)
	}
	oidcClientSecret := strings.TrimSpace(disk.Identity.ClientSecret)
	if oidcClientSecret == "" {
		oidcClientSecret = strings.TrimSpace(disk.Auth.ClientSecret)
	}
	if oidcClientSecret == "" {
		oidcClientSecret = strings.TrimSpace(disk.OIDCClientSecret)
	}
	casdoorOrg := strings.TrimSpace(disk.Identity.Organization)
	if casdoorOrg == "" {
		casdoorOrg = strings.TrimSpace(disk.Auth.Organization)
	}
	if casdoorOrg == "" {
		casdoorOrg = strings.TrimSpace(disk.CasdoorOrg)
	}
	casdoorApplication := strings.TrimSpace(disk.Identity.Application)
	if casdoorApplication == "" {
		casdoorApplication = strings.TrimSpace(disk.Auth.Application)
	}
	if casdoorApplication == "" {
		casdoorApplication = strings.TrimSpace(disk.CasdoorApplication)
	}

	return Config{
		LastProjectID:      projectID,
		LastProjectSlug:    projectSlug,
		MutagenDestination: mutagenDestination,
		MutagenSessionName: mutagenSessionName,
		LastVMName:         lastVMName,
		LastVMHTTPSURL:     lastVMHTTPSURL,
		LastVMSshDest:      lastVMSshDest,
		ProjectAPIToken:    strings.TrimSpace(disk.ProjectAPIToken),
		ProjectDBName:      projectDBName,
		ProjectDBUser:      projectDBUser,
		ProjectDBPassword:  projectDBPassword,
		ProjectDBHost:      projectDBHost,
		ProjectDBPort:      projectDBPort,
		ProjectDatabaseURL: databaseURL,
		WorkspaceBranch:    workspaceBranch,
		OIDCIssuer:         oidcIssuer,
		OIDCClientID:       oidcClientID,
		OIDCClientSecret:   oidcClientSecret,
		CasdoorOrg:         casdoorOrg,
		CasdoorApplication: casdoorApplication,
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
	onDisk.Auth.Issuer = strings.TrimSpace(cfg.OIDCIssuer)
	onDisk.Auth.ClientID = strings.TrimSpace(cfg.OIDCClientID)
	onDisk.Auth.ClientSecret = strings.TrimSpace(cfg.OIDCClientSecret)

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
