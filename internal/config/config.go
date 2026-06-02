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
	APIURL                    string `json:"apiUrl,omitempty"`
	Token                     string `json:"token,omitempty"`
	RefreshToken              string `json:"refreshToken,omitempty"`
	LastProjectID             string `json:"lastProjectId,omitempty"`
	LastProjectSlug           string `json:"lastProjectSlug,omitempty"`
	MutagenDestination        string `json:"mutagenDestination,omitempty"`
	MutagenSessionName        string `json:"mutagenSessionName,omitempty"`
	LastVMName                string `json:"lastVmName,omitempty"`
	LastVMHTTPSURL            string `json:"lastVmHttpsUrl,omitempty"`
	LastVMSshDest             string `json:"lastVmSshDest,omitempty"`
	ProjectAPIToken           string `json:"projectApiToken,omitempty"`
	ProjectDBName             string `json:"projectDbName,omitempty"`
	ProjectDBUser             string `json:"projectDbUser,omitempty"`
	ProjectDBPassword         string `json:"projectDbPassword,omitempty"`
	ProjectDBHost             string `json:"projectDbHost,omitempty"`
	ProjectDBPort             int    `json:"projectDbPort,omitempty"`
	ProjectDatabaseURL        string `json:"projectDatabaseUrl,omitempty"`
	WorkspaceBranch           string `json:"workspaceBranch,omitempty"`
	OIDCType                  string `json:"oidcType,omitempty"`
	OIDCProvider              string `json:"oidcProvider,omitempty"`
	OIDCIssuer                string `json:"oidcIssuer,omitempty"`
	OIDCDiscoveryURL          string `json:"oidcDiscoveryUrl,omitempty"`
	OIDCJWKSURI               string `json:"oidcJwksUri,omitempty"`
	OIDCAuthorizationEndpoint string `json:"oidcAuthorizationEndpoint,omitempty"`
	OIDCTokenEndpoint         string `json:"oidcTokenEndpoint,omitempty"`
	OIDCUserinfoEndpoint      string `json:"oidcUserinfoEndpoint,omitempty"`
	OIDCClientID              string `json:"oidcClientId,omitempty"`
	OIDCClientSecret          string `json:"oidcClientSecret,omitempty"`
	OIDCClientSecretRef       string `json:"oidcClientSecretRef,omitempty"`
	OIDCRedirectURI           string `json:"oidcRedirectUri,omitempty"`
	OIDCLogoutURI             string `json:"oidcLogoutUri,omitempty"`
	OIDCPostLogoutRedirectURI string `json:"oidcPostLogoutRedirectUri,omitempty"`
	OIDCScopes                string `json:"oidcScopes,omitempty"`
	OIDCLoginURL              string `json:"oidcLoginUrl,omitempty"`
	CasdoorOrg                string `json:"casdoorOrg,omitempty"`
	CasdoorApplication        string `json:"casdoorApplication,omitempty"`
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
		Type                  string   `json:"type,omitempty"`
		Provider              string   `json:"provider,omitempty"`
		Issuer                string   `json:"issuer,omitempty"`
		DiscoveryURL          string   `json:"discoveryUrl,omitempty"`
		JWKSURI               string   `json:"jwksUri,omitempty"`
		AuthorizationEndpoint string   `json:"authorizationEndpoint,omitempty"`
		TokenEndpoint         string   `json:"tokenEndpoint,omitempty"`
		UserinfoEndpoint      string   `json:"userinfoEndpoint,omitempty"`
		ClientID              string   `json:"clientId,omitempty"`
		ClientSecret          string   `json:"clientSecret,omitempty"`
		ClientSecretRef       string   `json:"clientSecretRef,omitempty"`
		RedirectURI           string   `json:"redirectUri,omitempty"`
		LogoutURI             string   `json:"logoutUri,omitempty"`
		PostLogoutRedirectURI string   `json:"postLogoutRedirectUri,omitempty"`
		Scopes                []string `json:"scopes,omitempty"`
		LoginURL              string   `json:"loginUrl,omitempty"`
		Organization          string   `json:"organization,omitempty"`
		Application           string   `json:"application,omitempty"`
	} `json:"auth,omitempty"`
	Identity struct {
		Type                  string   `json:"type,omitempty"`
		Provider              string   `json:"provider,omitempty"`
		Issuer                string   `json:"issuer,omitempty"`
		DiscoveryURL          string   `json:"discoveryUrl,omitempty"`
		JWKSURI               string   `json:"jwksUri,omitempty"`
		AuthorizationEndpoint string   `json:"authorizationEndpoint,omitempty"`
		TokenEndpoint         string   `json:"tokenEndpoint,omitempty"`
		UserinfoEndpoint      string   `json:"userinfoEndpoint,omitempty"`
		ClientID              string   `json:"clientId,omitempty"`
		ClientSecret          string   `json:"clientSecret,omitempty"`
		ClientSecretRef       string   `json:"clientSecretRef,omitempty"`
		RedirectURI           string   `json:"redirectUri,omitempty"`
		LogoutURI             string   `json:"logoutUri,omitempty"`
		PostLogoutRedirectURI string   `json:"postLogoutRedirectUri,omitempty"`
		Scopes                []string `json:"scopes,omitempty"`
		LoginURL              string   `json:"loginUrl,omitempty"`
		Organization          string   `json:"organization,omitempty"`
		Application           string   `json:"application,omitempty"`
	} `json:"identity,omitempty"`

	// Compatibilidad con formatos legacy flat.
	LastProjectID             string `json:"lastProjectId,omitempty"`
	LastProjectSlug           string `json:"lastProjectSlug,omitempty"`
	MutagenDestination        string `json:"mutagenDestination,omitempty"`
	MutagenSessionName        string `json:"mutagenSessionName,omitempty"`
	LastVMName                string `json:"lastVmName,omitempty"`
	LastVMHTTPSURL            string `json:"lastVmHttpsUrl,omitempty"`
	LastVMSshDest             string `json:"lastVmSshDest,omitempty"`
	ProjectAPIToken           string `json:"projectApiToken,omitempty"`
	ProjectDBName             string `json:"projectDbName,omitempty"`
	ProjectDBUser             string `json:"projectDbUser,omitempty"`
	ProjectDBPassword         string `json:"projectDbPassword,omitempty"`
	ProjectDBHost             string `json:"projectDbHost,omitempty"`
	ProjectDBPort             int    `json:"projectDbPort,omitempty"`
	ProjectDatabaseURL        string `json:"projectDatabaseUrl,omitempty"`
	WorkspaceBranch           string `json:"workspaceBranch,omitempty"`
	OIDCType                  string `json:"oidcType,omitempty"`
	OIDCProvider              string `json:"oidcProvider,omitempty"`
	OIDCIssuer                string `json:"oidcIssuer,omitempty"`
	OIDCDiscoveryURL          string `json:"oidcDiscoveryUrl,omitempty"`
	OIDCJWKSURI               string `json:"oidcJwksUri,omitempty"`
	OIDCAuthorizationEndpoint string `json:"oidcAuthorizationEndpoint,omitempty"`
	OIDCTokenEndpoint         string `json:"oidcTokenEndpoint,omitempty"`
	OIDCUserinfoEndpoint      string `json:"oidcUserinfoEndpoint,omitempty"`
	OIDCClientID              string `json:"oidcClientId,omitempty"`
	OIDCClientSecret          string `json:"oidcClientSecret,omitempty"`
	OIDCClientSecretRef       string `json:"oidcClientSecretRef,omitempty"`
	OIDCRedirectURI           string `json:"oidcRedirectUri,omitempty"`
	OIDCLogoutURI             string `json:"oidcLogoutUri,omitempty"`
	OIDCPostLogoutRedirectURI string `json:"oidcPostLogoutRedirectUri,omitempty"`
	OIDCScopes                string `json:"oidcScopes,omitempty"`
	OIDCLoginURL              string `json:"oidcLoginUrl,omitempty"`
	CasdoorOrg                string `json:"casdoorOrg,omitempty"`
	CasdoorApplication        string `json:"casdoorApplication,omitempty"`
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
	cfg.OIDCType = strings.TrimSpace(projectCfg.OIDCType)
	cfg.OIDCProvider = strings.TrimSpace(projectCfg.OIDCProvider)
	cfg.OIDCIssuer = strings.TrimSpace(projectCfg.OIDCIssuer)
	cfg.OIDCDiscoveryURL = strings.TrimSpace(projectCfg.OIDCDiscoveryURL)
	cfg.OIDCJWKSURI = strings.TrimSpace(projectCfg.OIDCJWKSURI)
	cfg.OIDCAuthorizationEndpoint = strings.TrimSpace(projectCfg.OIDCAuthorizationEndpoint)
	cfg.OIDCTokenEndpoint = strings.TrimSpace(projectCfg.OIDCTokenEndpoint)
	cfg.OIDCUserinfoEndpoint = strings.TrimSpace(projectCfg.OIDCUserinfoEndpoint)
	cfg.OIDCClientID = strings.TrimSpace(projectCfg.OIDCClientID)
	cfg.OIDCClientSecret = strings.TrimSpace(projectCfg.OIDCClientSecret)
	cfg.OIDCClientSecretRef = strings.TrimSpace(projectCfg.OIDCClientSecretRef)
	cfg.OIDCRedirectURI = strings.TrimSpace(projectCfg.OIDCRedirectURI)
	cfg.OIDCLogoutURI = strings.TrimSpace(projectCfg.OIDCLogoutURI)
	cfg.OIDCPostLogoutRedirectURI = strings.TrimSpace(projectCfg.OIDCPostLogoutRedirectURI)
	cfg.OIDCScopes = strings.TrimSpace(projectCfg.OIDCScopes)
	cfg.OIDCLoginURL = strings.TrimSpace(projectCfg.OIDCLoginURL)
	cfg.CasdoorOrg = strings.TrimSpace(projectCfg.CasdoorOrg)
	cfg.CasdoorApplication = strings.TrimSpace(projectCfg.CasdoorApplication)

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
	oidcType := firstNonEmpty(strings.TrimSpace(disk.Identity.Type), strings.TrimSpace(disk.Auth.Type), strings.TrimSpace(disk.OIDCType))
	oidcProvider := firstNonEmpty(strings.TrimSpace(disk.Identity.Provider), strings.TrimSpace(disk.Auth.Provider), strings.TrimSpace(disk.OIDCProvider))
	oidcIssuer := firstNonEmpty(strings.TrimSpace(disk.Identity.Issuer), strings.TrimSpace(disk.Auth.Issuer), strings.TrimSpace(disk.OIDCIssuer))
	oidcDiscoveryURL := firstNonEmpty(strings.TrimSpace(disk.Identity.DiscoveryURL), strings.TrimSpace(disk.Auth.DiscoveryURL), strings.TrimSpace(disk.OIDCDiscoveryURL))
	oidcJWKSURI := firstNonEmpty(strings.TrimSpace(disk.Identity.JWKSURI), strings.TrimSpace(disk.Auth.JWKSURI), strings.TrimSpace(disk.OIDCJWKSURI))
	oidcAuthorizationEndpoint := firstNonEmpty(strings.TrimSpace(disk.Identity.AuthorizationEndpoint), strings.TrimSpace(disk.Auth.AuthorizationEndpoint), strings.TrimSpace(disk.OIDCAuthorizationEndpoint))
	oidcTokenEndpoint := firstNonEmpty(strings.TrimSpace(disk.Identity.TokenEndpoint), strings.TrimSpace(disk.Auth.TokenEndpoint), strings.TrimSpace(disk.OIDCTokenEndpoint))
	oidcUserinfoEndpoint := firstNonEmpty(strings.TrimSpace(disk.Identity.UserinfoEndpoint), strings.TrimSpace(disk.Auth.UserinfoEndpoint), strings.TrimSpace(disk.OIDCUserinfoEndpoint))
	oidcClientID := firstNonEmpty(strings.TrimSpace(disk.Identity.ClientID), strings.TrimSpace(disk.Auth.ClientID), strings.TrimSpace(disk.OIDCClientID))
	oidcClientSecret := firstNonEmpty(strings.TrimSpace(disk.Identity.ClientSecret), strings.TrimSpace(disk.Auth.ClientSecret), strings.TrimSpace(disk.OIDCClientSecret))
	oidcClientSecretRef := firstNonEmpty(strings.TrimSpace(disk.Identity.ClientSecretRef), strings.TrimSpace(disk.Auth.ClientSecretRef), strings.TrimSpace(disk.OIDCClientSecretRef))
	oidcRedirectURI := firstNonEmpty(strings.TrimSpace(disk.Identity.RedirectURI), strings.TrimSpace(disk.Auth.RedirectURI), strings.TrimSpace(disk.OIDCRedirectURI))
	oidcLogoutURI := firstNonEmpty(strings.TrimSpace(disk.Identity.LogoutURI), strings.TrimSpace(disk.Auth.LogoutURI), strings.TrimSpace(disk.OIDCLogoutURI))
	oidcPostLogoutRedirectURI := firstNonEmpty(strings.TrimSpace(disk.Identity.PostLogoutRedirectURI), strings.TrimSpace(disk.Auth.PostLogoutRedirectURI), strings.TrimSpace(disk.OIDCPostLogoutRedirectURI))
	oidcScopes := strings.TrimSpace(disk.OIDCScopes)
	if oidcScopes == "" {
		if len(disk.Identity.Scopes) > 0 {
			oidcScopes = strings.Join(disk.Identity.Scopes, " ")
		} else if len(disk.Auth.Scopes) > 0 {
			oidcScopes = strings.Join(disk.Auth.Scopes, " ")
		}
	}
	oidcLoginURL := firstNonEmpty(strings.TrimSpace(disk.Identity.LoginURL), strings.TrimSpace(disk.Auth.LoginURL), strings.TrimSpace(disk.OIDCLoginURL))
	casdoorOrg := firstNonEmpty(strings.TrimSpace(disk.Identity.Organization), strings.TrimSpace(disk.Auth.Organization), strings.TrimSpace(disk.CasdoorOrg))
	casdoorApplication := firstNonEmpty(strings.TrimSpace(disk.Identity.Application), strings.TrimSpace(disk.Auth.Application), strings.TrimSpace(disk.CasdoorApplication))

	return Config{
		LastProjectID:             projectID,
		LastProjectSlug:           projectSlug,
		MutagenDestination:        mutagenDestination,
		MutagenSessionName:        mutagenSessionName,
		LastVMName:                lastVMName,
		LastVMHTTPSURL:            lastVMHTTPSURL,
		LastVMSshDest:             lastVMSshDest,
		ProjectAPIToken:           strings.TrimSpace(disk.ProjectAPIToken),
		ProjectDBName:             projectDBName,
		ProjectDBUser:             projectDBUser,
		ProjectDBPassword:         projectDBPassword,
		ProjectDBHost:             projectDBHost,
		ProjectDBPort:             projectDBPort,
		ProjectDatabaseURL:        databaseURL,
		WorkspaceBranch:           workspaceBranch,
		OIDCType:                  oidcType,
		OIDCProvider:              oidcProvider,
		OIDCIssuer:                oidcIssuer,
		OIDCDiscoveryURL:          oidcDiscoveryURL,
		OIDCJWKSURI:               oidcJWKSURI,
		OIDCAuthorizationEndpoint: oidcAuthorizationEndpoint,
		OIDCTokenEndpoint:         oidcTokenEndpoint,
		OIDCUserinfoEndpoint:      oidcUserinfoEndpoint,
		OIDCClientID:              oidcClientID,
		OIDCClientSecret:          oidcClientSecret,
		OIDCClientSecretRef:       oidcClientSecretRef,
		OIDCRedirectURI:           oidcRedirectURI,
		OIDCLogoutURI:             oidcLogoutURI,
		OIDCPostLogoutRedirectURI: oidcPostLogoutRedirectURI,
		OIDCScopes:                oidcScopes,
		OIDCLoginURL:              oidcLoginURL,
		CasdoorOrg:                casdoorOrg,
		CasdoorApplication:        casdoorApplication,
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
	onDisk.Auth.Type = strings.TrimSpace(cfg.OIDCType)
	onDisk.Auth.Provider = strings.TrimSpace(cfg.OIDCProvider)
	onDisk.Auth.Issuer = strings.TrimSpace(cfg.OIDCIssuer)
	onDisk.Auth.DiscoveryURL = strings.TrimSpace(cfg.OIDCDiscoveryURL)
	onDisk.Auth.JWKSURI = strings.TrimSpace(cfg.OIDCJWKSURI)
	onDisk.Auth.AuthorizationEndpoint = strings.TrimSpace(cfg.OIDCAuthorizationEndpoint)
	onDisk.Auth.TokenEndpoint = strings.TrimSpace(cfg.OIDCTokenEndpoint)
	onDisk.Auth.UserinfoEndpoint = strings.TrimSpace(cfg.OIDCUserinfoEndpoint)
	onDisk.Auth.ClientID = strings.TrimSpace(cfg.OIDCClientID)
	onDisk.Auth.ClientSecret = strings.TrimSpace(cfg.OIDCClientSecret)
	onDisk.Auth.ClientSecretRef = strings.TrimSpace(cfg.OIDCClientSecretRef)
	onDisk.Auth.RedirectURI = strings.TrimSpace(cfg.OIDCRedirectURI)
	onDisk.Auth.LogoutURI = strings.TrimSpace(cfg.OIDCLogoutURI)
	onDisk.Auth.PostLogoutRedirectURI = strings.TrimSpace(cfg.OIDCPostLogoutRedirectURI)
	if scopes := strings.Fields(strings.TrimSpace(cfg.OIDCScopes)); len(scopes) > 0 {
		onDisk.Auth.Scopes = scopes
	}
	onDisk.Auth.LoginURL = strings.TrimSpace(cfg.OIDCLoginURL)
	onDisk.Auth.Organization = strings.TrimSpace(cfg.CasdoorOrg)
	onDisk.Auth.Application = strings.TrimSpace(cfg.CasdoorApplication)

	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(onDisk); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0o600); err != nil {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(onDisk); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0o600); err != nil {
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
