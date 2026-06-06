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
	APIURL                     string `json:"apiUrl,omitempty"`
	Token                      string `json:"token,omitempty"`
	RefreshToken               string `json:"refreshToken,omitempty"`
	LastProjectID              string `json:"lastProjectId,omitempty"`
	LastProjectSlug            string `json:"lastProjectSlug,omitempty"`
	MutagenDestination         string `json:"mutagenDestination,omitempty"`
	MutagenSessionName         string `json:"mutagenSessionName,omitempty"`
	LastVMName                 string `json:"lastVmName,omitempty"`
	LastVMHTTPSURL             string `json:"lastVmHttpsUrl,omitempty"`
	LastVMSshDest              string `json:"lastVmSshDest,omitempty"`
	ProjectAPIToken            string `json:"projectApiToken,omitempty"`
	ProjectDBName              string `json:"projectDbName,omitempty"`
	ProjectDBUser              string `json:"projectDbUser,omitempty"`
	ProjectDBPassword          string `json:"projectDbPassword,omitempty"`
	ProjectDBHost              string `json:"projectDbHost,omitempty"`
	ProjectDBPort              int    `json:"projectDbPort,omitempty"`
	ProjectDatabaseURL         string `json:"projectDatabaseUrl,omitempty"`
	WorkspaceBranch            string `json:"workspaceBranch,omitempty"`
	OIDCType                   string `json:"oidcType,omitempty"`
	OIDCProvider               string `json:"oidcProvider,omitempty"`
	OIDCIssuer                 string `json:"oidcIssuer,omitempty"`
	OIDCDiscoveryURL           string `json:"oidcDiscoveryUrl,omitempty"`
	OIDCJWKSURI                string `json:"oidcJwksUri,omitempty"`
	OIDCAuthorizationEndpoint  string `json:"oidcAuthorizationEndpoint,omitempty"`
	OIDCTokenEndpoint          string `json:"oidcTokenEndpoint,omitempty"`
	OIDCUserinfoEndpoint       string `json:"oidcUserinfoEndpoint,omitempty"`
	OIDCClientID               string `json:"oidcClientId,omitempty"`
	OIDCClientSecret           string `json:"oidcClientSecret,omitempty"`
	OIDCClientSecretRef        string `json:"oidcClientSecretRef,omitempty"`
	OIDCRedirectURI            string `json:"oidcRedirectUri,omitempty"`
	OIDCLogoutURI              string `json:"oidcLogoutUri,omitempty"`
	OIDCPostLogoutRedirectURI  string `json:"oidcPostLogoutRedirectUri,omitempty"`
	OIDCScopes                 string `json:"oidcScopes,omitempty"`
	OIDCLoginURL               string `json:"oidcLoginUrl,omitempty"`
	CasdoorOrg                 string `json:"casdoorOrg,omitempty"`
	CasdoorApplication         string `json:"casdoorApplication,omitempty"`
	MachineAuthGrantType       string `json:"machineAuthGrantType,omitempty"`
	MachineAuthTokenEndpoint   string `json:"machineAuthTokenEndpoint,omitempty"`
	MachineAuthClientID        string `json:"machineAuthClientId,omitempty"`
	MachineAuthClientSecret    string `json:"machineAuthClientSecret,omitempty"`
	MachineAuthClientSecretRef string `json:"machineAuthClientSecretRef,omitempty"`
	MachineAuthAudience        string `json:"machineAuthAudience,omitempty"`
	MachineAuthScopes          string `json:"machineAuthScopes,omitempty"`
}

type projectDiskConfig struct {
	Project struct {
		ID   string `json:"id,omitempty"`
		Slug string `json:"slug,omitempty"`
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
		Machine               struct {
			GrantType       string   `json:"grantType,omitempty"`
			TokenEndpoint   string   `json:"tokenEndpoint,omitempty"`
			ClientID        string   `json:"clientId,omitempty"`
			ClientSecret    string   `json:"clientSecret,omitempty"`
			ClientSecretRef string   `json:"clientSecretRef,omitempty"`
			Audience        string   `json:"audience,omitempty"`
			Scopes          []string `json:"scopes,omitempty"`
		} `json:"machine,omitempty"`
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
	MachineAuth struct {
		GrantType       string   `json:"grantType,omitempty"`
		TokenEndpoint   string   `json:"tokenEndpoint,omitempty"`
		ClientID        string   `json:"clientId,omitempty"`
		ClientSecret    string   `json:"clientSecret,omitempty"`
		ClientSecretRef string   `json:"clientSecretRef,omitempty"`
		Audience        string   `json:"audience,omitempty"`
		Scopes          []string `json:"scopes,omitempty"`
	} `json:"machineAuth,omitempty"`

	// Compatibilidad con formatos legacy flat.
	LastProjectID              string `json:"lastProjectId,omitempty"`
	LastProjectSlug            string `json:"lastProjectSlug,omitempty"`
	MutagenDestination         string `json:"mutagenDestination,omitempty"`
	MutagenSessionName         string `json:"mutagenSessionName,omitempty"`
	LastVMName                 string `json:"lastVmName,omitempty"`
	LastVMHTTPSURL             string `json:"lastVmHttpsUrl,omitempty"`
	LastVMSshDest              string `json:"lastVmSshDest,omitempty"`
	ProjectAPIToken            string `json:"projectApiToken,omitempty"`
	ProjectDBName              string `json:"projectDbName,omitempty"`
	ProjectDBUser              string `json:"projectDbUser,omitempty"`
	ProjectDBPassword          string `json:"projectDbPassword,omitempty"`
	ProjectDBHost              string `json:"projectDbHost,omitempty"`
	ProjectDBPort              int    `json:"projectDbPort,omitempty"`
	ProjectDatabaseURL         string `json:"projectDatabaseUrl,omitempty"`
	WorkspaceBranch            string `json:"workspaceBranch,omitempty"`
	OIDCType                   string `json:"oidcType,omitempty"`
	OIDCProvider               string `json:"oidcProvider,omitempty"`
	OIDCIssuer                 string `json:"oidcIssuer,omitempty"`
	OIDCDiscoveryURL           string `json:"oidcDiscoveryUrl,omitempty"`
	OIDCJWKSURI                string `json:"oidcJwksUri,omitempty"`
	OIDCAuthorizationEndpoint  string `json:"oidcAuthorizationEndpoint,omitempty"`
	OIDCTokenEndpoint          string `json:"oidcTokenEndpoint,omitempty"`
	OIDCUserinfoEndpoint       string `json:"oidcUserinfoEndpoint,omitempty"`
	OIDCClientID               string `json:"oidcClientId,omitempty"`
	OIDCClientSecret           string `json:"oidcClientSecret,omitempty"`
	OIDCClientSecretRef        string `json:"oidcClientSecretRef,omitempty"`
	OIDCRedirectURI            string `json:"oidcRedirectUri,omitempty"`
	OIDCLogoutURI              string `json:"oidcLogoutUri,omitempty"`
	OIDCPostLogoutRedirectURI  string `json:"oidcPostLogoutRedirectUri,omitempty"`
	OIDCScopes                 string `json:"oidcScopes,omitempty"`
	OIDCLoginURL               string `json:"oidcLoginUrl,omitempty"`
	CasdoorOrg                 string `json:"casdoorOrg,omitempty"`
	CasdoorApplication         string `json:"casdoorApplication,omitempty"`
	MachineAuthGrantType       string `json:"machineAuthGrantType,omitempty"`
	MachineAuthTokenEndpoint   string `json:"machineAuthTokenEndpoint,omitempty"`
	MachineAuthClientID        string `json:"machineAuthClientId,omitempty"`
	MachineAuthClientSecret    string `json:"machineAuthClientSecret,omitempty"`
	MachineAuthClientSecretRef string `json:"machineAuthClientSecretRef,omitempty"`
	MachineAuthAudience        string `json:"machineAuthAudience,omitempty"`
	MachineAuthScopes          string `json:"machineAuthScopes,omitempty"`
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
	cfg.MachineAuthGrantType = strings.TrimSpace(projectCfg.MachineAuthGrantType)
	cfg.MachineAuthTokenEndpoint = strings.TrimSpace(projectCfg.MachineAuthTokenEndpoint)
	cfg.MachineAuthClientID = strings.TrimSpace(projectCfg.MachineAuthClientID)
	cfg.MachineAuthClientSecret = strings.TrimSpace(projectCfg.MachineAuthClientSecret)
	cfg.MachineAuthClientSecretRef = strings.TrimSpace(projectCfg.MachineAuthClientSecretRef)
	cfg.MachineAuthAudience = strings.TrimSpace(projectCfg.MachineAuthAudience)
	cfg.MachineAuthScopes = strings.TrimSpace(projectCfg.MachineAuthScopes)

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
	projectSlug := strings.TrimSpace(disk.Project.Slug)
	if projectSlug == "" {
		projectSlug = strings.TrimSpace(disk.Project.Name)
	}
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
	if mutagenDestination == "" || mutagenSessionName == "" {
		if sessionName, destination, err := loadMutagenConfigFromFile(); err == nil {
			if mutagenSessionName == "" {
				mutagenSessionName = sessionName
			}
			if mutagenDestination == "" {
				mutagenDestination = destination
			}
		}
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
	if lastVMSshDest == "" {
		lastVMSshDest = mutagenDestination
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
	machineAuthGrantType := firstNonEmpty(strings.TrimSpace(disk.Auth.Machine.GrantType), strings.TrimSpace(disk.MachineAuth.GrantType), strings.TrimSpace(disk.MachineAuthGrantType))
	machineAuthTokenEndpoint := firstNonEmpty(strings.TrimSpace(disk.Auth.Machine.TokenEndpoint), strings.TrimSpace(disk.MachineAuth.TokenEndpoint), strings.TrimSpace(disk.MachineAuthTokenEndpoint), oidcTokenEndpoint)
	machineAuthClientID := firstNonEmpty(strings.TrimSpace(disk.Auth.Machine.ClientID), strings.TrimSpace(disk.MachineAuth.ClientID), strings.TrimSpace(disk.MachineAuthClientID), oidcClientID)
	machineAuthClientSecret := firstNonEmpty(strings.TrimSpace(disk.Auth.Machine.ClientSecret), strings.TrimSpace(disk.MachineAuth.ClientSecret), strings.TrimSpace(disk.MachineAuthClientSecret))
	machineAuthClientSecretRef := firstNonEmpty(strings.TrimSpace(disk.Auth.Machine.ClientSecretRef), strings.TrimSpace(disk.MachineAuth.ClientSecretRef), strings.TrimSpace(disk.MachineAuthClientSecretRef))
	machineAuthAudience := firstNonEmpty(strings.TrimSpace(disk.Auth.Machine.Audience), strings.TrimSpace(disk.MachineAuth.Audience), strings.TrimSpace(disk.MachineAuthAudience))
	machineAuthScopes := strings.TrimSpace(disk.MachineAuthScopes)
	if machineAuthScopes == "" {
		if len(disk.Auth.Machine.Scopes) > 0 {
			machineAuthScopes = strings.Join(disk.Auth.Machine.Scopes, " ")
		} else if len(disk.MachineAuth.Scopes) > 0 {
			machineAuthScopes = strings.Join(disk.MachineAuth.Scopes, " ")
		}
	}

	return Config{
		LastProjectID:              projectID,
		LastProjectSlug:            projectSlug,
		MutagenDestination:         mutagenDestination,
		MutagenSessionName:         mutagenSessionName,
		LastVMName:                 lastVMName,
		LastVMHTTPSURL:             lastVMHTTPSURL,
		LastVMSshDest:              lastVMSshDest,
		ProjectAPIToken:            strings.TrimSpace(disk.ProjectAPIToken),
		ProjectDBName:              projectDBName,
		ProjectDBUser:              projectDBUser,
		ProjectDBPassword:          projectDBPassword,
		ProjectDBHost:              projectDBHost,
		ProjectDBPort:              projectDBPort,
		ProjectDatabaseURL:         databaseURL,
		WorkspaceBranch:            workspaceBranch,
		OIDCType:                   oidcType,
		OIDCProvider:               oidcProvider,
		OIDCIssuer:                 oidcIssuer,
		OIDCDiscoveryURL:           oidcDiscoveryURL,
		OIDCJWKSURI:                oidcJWKSURI,
		OIDCAuthorizationEndpoint:  oidcAuthorizationEndpoint,
		OIDCTokenEndpoint:          oidcTokenEndpoint,
		OIDCUserinfoEndpoint:       oidcUserinfoEndpoint,
		OIDCClientID:               oidcClientID,
		OIDCClientSecret:           oidcClientSecret,
		OIDCClientSecretRef:        oidcClientSecretRef,
		OIDCRedirectURI:            oidcRedirectURI,
		OIDCLogoutURI:              oidcLogoutURI,
		OIDCPostLogoutRedirectURI:  oidcPostLogoutRedirectURI,
		OIDCScopes:                 oidcScopes,
		OIDCLoginURL:               oidcLoginURL,
		CasdoorOrg:                 casdoorOrg,
		CasdoorApplication:         casdoorApplication,
		MachineAuthGrantType:       machineAuthGrantType,
		MachineAuthTokenEndpoint:   machineAuthTokenEndpoint,
		MachineAuthClientID:        machineAuthClientID,
		MachineAuthClientSecret:    machineAuthClientSecret,
		MachineAuthClientSecretRef: machineAuthClientSecretRef,
		MachineAuthAudience:        machineAuthAudience,
		MachineAuthScopes:          machineAuthScopes,
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

	onDisk := map[string]any{}

	if projectID := strings.TrimSpace(cfg.LastProjectID); projectID != "" || strings.TrimSpace(cfg.LastProjectSlug) != "" {
		project := map[string]any{}
		if projectID != "" {
			project["id"] = projectID
		}
		if slug := strings.TrimSpace(cfg.LastProjectSlug); slug != "" {
			project["slug"] = slug
		}
		onDisk["project"] = project
	}
	if branch := strings.TrimSpace(cfg.WorkspaceBranch); branch != "" {
		onDisk["workspace"] = map[string]any{"branch": branch}
	}
	if httpsURL := strings.TrimSpace(cfg.LastVMHTTPSURL); httpsURL != "" {
		onDisk["vm"] = map[string]any{"httpsUrl": httpsURL}
	}
	if databaseURL := strings.TrimSpace(cfg.ProjectDatabaseURL); databaseURL != "" {
		onDisk["database"] = map[string]any{"url": databaseURL}
	}

	auth := map[string]any{}
	setIf := func(target map[string]any, key, value string) {
		if v := strings.TrimSpace(value); v != "" {
			target[key] = v
		}
	}
	setIf(auth, "type", cfg.OIDCType)
	setIf(auth, "provider", cfg.OIDCProvider)
	setIf(auth, "issuer", cfg.OIDCIssuer)
	setIf(auth, "discoveryUrl", cfg.OIDCDiscoveryURL)
	setIf(auth, "jwksUri", cfg.OIDCJWKSURI)
	setIf(auth, "authorizationEndpoint", cfg.OIDCAuthorizationEndpoint)
	setIf(auth, "tokenEndpoint", cfg.OIDCTokenEndpoint)
	setIf(auth, "userinfoEndpoint", cfg.OIDCUserinfoEndpoint)
	setIf(auth, "clientId", cfg.OIDCClientID)
	setIf(auth, "redirectUri", cfg.OIDCRedirectURI)
	setIf(auth, "logoutUri", cfg.OIDCLogoutURI)
	setIf(auth, "postLogoutRedirectUri", cfg.OIDCPostLogoutRedirectURI)
	if scopes := strings.Fields(strings.TrimSpace(cfg.OIDCScopes)); len(scopes) > 0 {
		auth["scopes"] = scopes
	}
	setIf(auth, "loginUrl", cfg.OIDCLoginURL)
	setIf(auth, "organization", cfg.CasdoorOrg)
	setIf(auth, "application", cfg.CasdoorApplication)

	machine := map[string]any{}
	setIf(machine, "grantType", cfg.MachineAuthGrantType)
	setIf(machine, "clientSecret", cfg.MachineAuthClientSecret)
	setIf(machine, "clientSecretRef", cfg.MachineAuthClientSecretRef)
	setIf(machine, "audience", cfg.MachineAuthAudience)
	if scopes := strings.Fields(strings.TrimSpace(cfg.MachineAuthScopes)); len(scopes) > 0 {
		machine["scopes"] = scopes
	}
	if len(machine) > 0 {
		auth["machine"] = machine
	}
	if len(auth) > 0 {
		onDisk["auth"] = auth
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

func loadMutagenConfigFromFile() (string, string, error) {
	b, err := os.ReadFile("mutagen.yml")
	if err != nil {
		return "", "", err
	}

	var currentSession string
	inSync := false
	inDefaults := false
	sessionName := ""
	destination := ""

	for _, raw := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "sync:" {
			inSync = true
			continue
		}
		if !inSync {
			continue
		}
		if !strings.HasPrefix(raw, "  ") {
			break
		}
		if strings.HasPrefix(raw, "  defaults:") {
			inDefaults = true
			currentSession = ""
			continue
		}
		if strings.HasPrefix(raw, "  ") && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(raw, "    ") {
			name := strings.TrimSuffix(trimmed, ":")
			if name == "defaults" {
				inDefaults = true
				currentSession = ""
				continue
			}
			if sessionName == "" {
				sessionName = name
			}
			currentSession = name
			inDefaults = false
			continue
		}
		if inDefaults || currentSession == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "beta:") {
			destination = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "beta:")), "\"'")
			break
		}
	}

	if sessionName == "" || destination == "" {
		return "", "", fmt.Errorf("mutagen.yml incompleto")
	}
	return sessionName, destination, nil
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
