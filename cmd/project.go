package cmd

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/api"
	"github.com/Ignaciojeria/sync/internal/config"
	"github.com/Ignaciojeria/sync/internal/scaffold"

	"github.com/spf13/cobra"
)

var (
	skipMutagenCheck       bool
	initMutagenDestination string
	initMutagenName        string
	skipMutagenStart       bool
	skipSSHOnboarding      bool
	skipAirAutoStart       bool
	skipInitialCommit      bool
	forceWorkspaceTakeover bool
)

var initCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Inicializa un proyecto en Einar (entrypoint API en cmd/api/main.go)",
	Long:  "Inicializa un proyecto Go base en Einar. El scaffold recomendado crea el entrypoint de la API en cmd/api/main.go para separar runtime de API y comandos CLI.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		if name == "" {
			return fmt.Errorf("nombre de proyecto requerido")
		}

		cfg, err := config.Resolve(apiURLFlag, tokenFlag)
		if err != nil {
			return err
		}
		if cfg.Token == "" {
			return fmt.Errorf("falta token (usa 'login' o EINAR_TOKEN)")
		}

		if debugHTTP {
			fmt.Println("[debug] HTTP request")
			fmt.Printf("[debug] POST %s/api/projects\n", cfg.APIURL)
			fmt.Printf("[debug] Authorization: Bearer %s\n", config.MaskToken(cfg.Token))
			fmt.Printf("[debug] Body: {\"name\":%q,\"public\":true,\"visibility\":\"public\"}\n", name)
		}

		client := api.NewClient(cfg.APIURL, cfg.Token, 10*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		resp, err := client.CreateProject(ctx, name)
		if err != nil {
			if refreshed, rerr := shouldRefreshAndRetry(err, &cfg); rerr != nil {
				return rerr
			} else if refreshed {
				client = api.NewClient(cfg.APIURL, cfg.Token, 10*time.Second)
				resp, err = client.CreateProject(ctx, name)
			}
		}
		if err != nil {
			if debugHTTP {
				if ae := (&api.APIError{}); api.AsAPIError(err, ae) {
					fmt.Printf("[debug] HTTP error status=%d code=%q message=%q\n", ae.StatusCode, ae.Code, ae.Message)
					if strings.TrimSpace(ae.RawBody) != "" {
						fmt.Printf("[debug] HTTP error body: %s\n", ae.RawBody)
					}
				} else {
					fmt.Printf("[debug] HTTP transport error: %v\n", err)
				}
			}
			if msg := mapAPIError(err); msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return err
		}
		if debugHTTP {
			if b, merr := json.MarshalIndent(resp, "", "  "); merr == nil {
				fmt.Printf("[debug] Response body:\n%s\n", string(b))
			}
		}
		projectID := strings.TrimSpace(resp.ProjectID)
		slug := strings.TrimSpace(resp.Slug)
		if projectID == "" || slug == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: projectId/slug vacíos")
		}
		normalizedMachineAuth := normalizeProjectMachineAuth(resp)
		postgresMachineClientID := ""
		if normalizedMachineAuth != nil {
			postgresMachineClientID = strings.TrimSpace(normalizedMachineAuth.ClientID)
		}
		postgresProject, err := ensurePostgresProjectProvisioned(ctx, &cfg, slug, postgresMachineClientID)
		if err != nil {
			return err
		}

		localProjectDir := slug
		if localProjectDir == "" {
			localProjectDir = strings.TrimSpace(name)
		}
		if err := os.MkdirAll(localProjectDir, 0o755); err != nil {
			return fmt.Errorf("no se pudo crear carpeta local del proyecto: %w", err)
		}
		if err := os.Chdir(localProjectDir); err != nil {
			return fmt.Errorf("no se pudo entrar a la carpeta local del proyecto: %w", err)
		}

		bootstrapEmail := extractEmailFromJWT(cfg.Token)
		if err := ensureGoProjectScaffold(slug, bootstrapEmail); err != nil {
			return fmt.Errorf("no se pudo preparar scaffold Go base: %w", err)
		}
		if err := ensureProjectGitRepository(); err != nil {
			return fmt.Errorf("no se pudo inicializar repositorio git del proyecto: %w", err)
		}
		if err := ensureLocalAirConfig(); err != nil {
			return fmt.Errorf("no se pudo preparar .air.toml base: %w", err)
		}
		if err := ensureWorkspaceConfig(slug); err != nil {
			return fmt.Errorf("no se pudo preparar workspaces.yaml base: %w", err)
		}
		if err := ensureProjectGitIgnore(); err != nil {
			return fmt.Errorf("no se pudo preparar .gitignore base: %w", err)
		}
		if err := ensureGitHooksForWorkspaceLock(); err != nil {
			return fmt.Errorf("no se pudo preparar hooks git de workspace lock: %w", err)
		}
		if err := ensureInitialGitCommit(skipInitialCommit); err != nil {
			return fmt.Errorf("no se pudo crear commit inicial del proyecto: %w", err)
		}
		if currentBranch, ok := currentGitBranch(); ok {
			cfg.WorkspaceBranch = currentBranch
		} else if strings.TrimSpace(cfg.WorkspaceBranch) == "" {
			cfg.WorkspaceBranch = "main"
		}
		fmt.Println("ℹ️  Workspaces base configurados en workspaces.yaml (prod/main y develop/develop)")
		if isGitRepository() && !gitBranchExists("develop") {
			fmt.Println("ℹ️  Recomendación: crea la branch 'develop' si usarás flujo GitFlow")
		}

		cfg.LastProjectID = projectID
		cfg.LastProjectSlug = slug
		cfg.MutagenDestination = strings.TrimSpace(resp.Sync.Destination)
		cfg.MutagenSessionName = strings.TrimSpace(resp.Sync.SessionName)
		cfg.LastVMName = strings.TrimSpace(resp.VM.Name)
		cfg.LastVMHTTPSURL = strings.TrimSpace(resp.VM.HTTPSURL)
		cfg.LastVMSshDest = strings.TrimSpace(resp.VM.SSHDestination)
		cfg.WorkspaceBranch = strings.TrimSpace(resp.Workspace.Branch)
		projectAPIToken := strings.TrimSpace(resp.ProjectAPIToken)
		dbPassword := strings.TrimSpace(resp.DBPassword)
		sshPrivateKey := strings.TrimSpace(resp.VMSshPrivateKey)
		if resp.Secrets != nil {
			if v := strings.TrimSpace(resp.Secrets.ProjectAPIToken); v != "" {
				projectAPIToken = v
			}
			if v := strings.TrimSpace(resp.Secrets.DBPassword); v != "" {
				dbPassword = v
			}
			if v := strings.TrimSpace(resp.Secrets.SSHPrivateKey); v != "" {
				sshPrivateKey = v
			}
		}
		if projectAPIToken != "" {
			cfg.ProjectAPIToken = projectAPIToken
		}
		cfg.ProjectDBName = strings.TrimSpace(resp.Database.Name)
		cfg.ProjectDBUser = strings.TrimSpace(resp.Database.User)
		cfg.ProjectDBPassword = dbPassword
		cfg.ProjectDBHost = strings.TrimSpace(resp.Database.Host)
		if resp.Database.Port > 0 {
			cfg.ProjectDBPort = resp.Database.Port
		}
		cfg.ProjectDatabaseURL = strings.TrimSpace(resp.DatabaseURL)
		if postgresProject != nil {
			if strings.TrimSpace(postgresProject.DatabaseURL) != "" {
				cfg.ProjectDatabaseURL = strings.TrimSpace(postgresProject.DatabaseURL)
			}
			if strings.TrimSpace(postgresProject.Schema) != "" {
				cfg.ProjectDBName = strings.TrimSpace(postgresProject.Schema)
			}
		}
		if cfg.ProjectDatabaseURL == "" {
			cfg.ProjectDatabaseURL = buildDatabaseURL(cfg.ProjectDBUser, cfg.ProjectDBPassword, cfg.ProjectDBHost, cfg.ProjectDBPort, cfg.ProjectDBName)
		}
		normalizedAuth := normalizeProjectAuth(resp)
		if normalizedAuth != nil {
			cfg.OIDCType = strings.TrimSpace(normalizedAuth.Type)
			cfg.OIDCProvider = strings.TrimSpace(normalizedAuth.Provider)
			cfg.OIDCIssuer = strings.TrimSpace(normalizedAuth.Issuer)
			cfg.OIDCDiscoveryURL = strings.TrimSpace(normalizedAuth.DiscoveryURL)
			cfg.OIDCJWKSURI = strings.TrimSpace(normalizedAuth.JWKSURI)
			cfg.OIDCAuthorizationEndpoint = strings.TrimSpace(normalizedAuth.AuthorizationEndpoint)
			cfg.OIDCTokenEndpoint = strings.TrimSpace(normalizedAuth.TokenEndpoint)
			cfg.OIDCUserinfoEndpoint = strings.TrimSpace(normalizedAuth.UserinfoEndpoint)
			cfg.OIDCClientID = strings.TrimSpace(normalizedAuth.ClientID)
			cfg.OIDCClientSecret = strings.TrimSpace(normalizedAuth.ClientSecret)
			cfg.OIDCClientSecretRef = strings.TrimSpace(normalizedAuth.ClientSecretRef)
			cfg.OIDCRedirectURI = strings.TrimSpace(normalizedAuth.RedirectURI)
			cfg.OIDCLogoutURI = strings.TrimSpace(normalizedAuth.LogoutURI)
			cfg.OIDCPostLogoutRedirectURI = strings.TrimSpace(normalizedAuth.PostLogoutRedirectURI)
			cfg.OIDCScopes = strings.Join(normalizedAuth.Scopes, " ")
			cfg.OIDCLoginURL = strings.TrimSpace(normalizedAuth.LoginURL)
			cfg.OIDCGoogleLoginURL = strings.TrimSpace(normalizedAuth.GoogleLoginURL)
			cfg.OIDCUpstreamGoogleClientID = strings.TrimSpace(normalizedAuth.UpstreamGoogleClientID)
			cfg.CasdoorOrg = strings.TrimSpace(normalizedAuth.Organization)
			cfg.CasdoorApplication = strings.TrimSpace(normalizedAuth.Application)
			if cfg.OIDCClientSecret == "" && cfg.OIDCClientSecretRef != "" {
				fmt.Printf("⚠️  OIDC secret no vino inline; secretRef=%s\n", cfg.OIDCClientSecretRef)
			}
		}
		if normalizedMachineAuth != nil {
			cfg.MachineAuthGrantType = strings.TrimSpace(normalizedMachineAuth.GrantType)
			cfg.MachineAuthTokenEndpoint = strings.TrimSpace(normalizedMachineAuth.TokenEndpoint)
			cfg.MachineAuthClientID = strings.TrimSpace(normalizedMachineAuth.ClientID)
			cfg.MachineAuthClientSecret = strings.TrimSpace(normalizedMachineAuth.ClientSecret)
			cfg.MachineAuthClientSecretRef = strings.TrimSpace(normalizedMachineAuth.ClientSecretRef)
			cfg.MachineAuthAudience = strings.TrimSpace(normalizedMachineAuth.Audience)
			cfg.MachineAuthScopes = strings.Join(normalizedMachineAuth.Scopes, " ")
			if cfg.MachineAuthClientSecret == "" && cfg.MachineAuthClientSecretRef != "" {
				fmt.Printf("⚠️  Machine auth secret no vino inline; secretRef=%s\n", cfg.MachineAuthClientSecretRef)
			}
		}
		normalizedCasdoorAdmin := normalizeProjectCasdoorAdmin(resp)
		if normalizedCasdoorAdmin != nil {
			cfg.CasdoorAdminAPIBaseURL = strings.TrimSpace(normalizedCasdoorAdmin.APIBaseURL)
			cfg.CasdoorAdminGatewayURL = strings.TrimSpace(normalizedCasdoorAdmin.GatewayURL)
			cfg.CasdoorOrg = firstNonEmptyTrimmed(strings.TrimSpace(cfg.CasdoorOrg), strings.TrimSpace(normalizedCasdoorAdmin.Organization))
			cfg.CasdoorApplication = firstNonEmptyTrimmed(strings.TrimSpace(cfg.CasdoorApplication), strings.TrimSpace(normalizedCasdoorAdmin.Application))
			cfg.CasdoorAdminClientID = strings.TrimSpace(normalizedCasdoorAdmin.ClientID)
			cfg.CasdoorAdminClientSecret = strings.TrimSpace(normalizedCasdoorAdmin.ClientSecret)
			cfg.CasdoorAdminClientSecretRef = strings.TrimSpace(normalizedCasdoorAdmin.ClientSecretRef)
			cfg.CasdoorAdminTokenEndpoint = strings.TrimSpace(normalizedCasdoorAdmin.TokenEndpoint)
			cfg.CasdoorAdminScopes = strings.Join(normalizedCasdoorAdmin.Scopes, " ")
			cfg.CasdoorAdminTenantScopedOnly = normalizedCasdoorAdmin.TenantScopedOnly
			if cfg.CasdoorAdminClientSecret == "" && cfg.CasdoorAdminClientSecretRef != "" {
				fmt.Printf("⚠️  Casdoor admin secret no vino inline; secretRef=%s\n", cfg.CasdoorAdminClientSecretRef)
			}
		}
		normalizedAIGateway := normalizeProjectAIGateway(resp)
		if normalizedAIGateway != nil {
			cfg.AIGatewayProvider = strings.TrimSpace(normalizedAIGateway.Provider)
			cfg.AIGatewayAPIBaseURL = strings.TrimSpace(normalizedAIGateway.APIBaseURL)
			cfg.AIGatewayClientID = strings.TrimSpace(normalizedAIGateway.ClientID)
			cfg.AIGatewayClientName = strings.TrimSpace(normalizedAIGateway.ClientName)
			cfg.AIGatewayClientEmail = strings.TrimSpace(normalizedAIGateway.ClientEmail)
			cfg.AIGatewayKeyLabel = strings.TrimSpace(normalizedAIGateway.KeyLabel)
			cfg.AIGatewayKeyID = strings.TrimSpace(normalizedAIGateway.KeyID)
			cfg.AIGatewayKeyPrefix = strings.TrimSpace(normalizedAIGateway.KeyPrefix)
			cfg.AIGatewayAPIKey = strings.TrimSpace(normalizedAIGateway.APIKey)
			cfg.AIGatewayAPIKeyRef = strings.TrimSpace(normalizedAIGateway.APIKeyRef)
			if cfg.AIGatewayAPIKey == "" && cfg.AIGatewayAPIKeyRef != "" {
				fmt.Printf("⚠️  AI gateway apiKey no vino inline; apiKeyRef=%s\n", cfg.AIGatewayAPIKeyRef)
			}
		}
		if sshPrivateKey != "" {
			fmt.Println("✅ Clave SSH de VM recibida inline desde backend")
			if err := writeSSHPrivateKey(slug, cfg.MutagenDestination, sshPrivateKey); err != nil {
				return fmt.Errorf("no se pudo guardar clave SSH privada: %w", err)
			}
		} else {
			fmt.Println("ℹ️  Backend no devolvió sshPrivateKey inline; se usará flujo SSH legacy (puede requerir onboarding exe.dev)")
		}
		if err := writeProjectSecretsBundle(slug, sshPrivateKey, projectAPIToken, dbPassword, cfg.MachineAuthClientID, cfg.MachineAuthClientSecret, cfg.CasdoorAdminClientSecret, cfg.AIGatewayAPIKey); err != nil {
			return fmt.Errorf("no se pudieron guardar secretos locales del proyecto: %w", err)
		}
		if strings.TrimSpace(cfg.MutagenDestination) == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: sync.destination vacío")
		}
		if strings.TrimSpace(cfg.MutagenSessionName) == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: sync.sessionName vacío")
		}
		if strings.TrimSpace(cfg.LastVMSshDest) == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: vm.sshDestination vacío")
		}
		if strings.TrimSpace(cfg.WorkspaceBranch) == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: workspace.branch vacío")
		}

		if err := ensureWorkspaceOwnership(&cfg, forceWorkspaceTakeover); err != nil {
			return err
		}
		if err := saveProjectConfig(cfg); err != nil {
			return fmt.Errorf("no se pudo guardar config local: %w", err)
		}
		if err := materializeProjectEnv(cfg); err != nil {
			return fmt.Errorf("no se pudo materializar .env del proyecto: %w", err)
		}
		if err := ensureGoModTidy(); err != nil {
			return fmt.Errorf("no se pudo ejecutar go mod tidy: %w", err)
		}
		if strings.TrimSpace(cfg.ProjectDatabaseURL) != "" {
			fmt.Println("🚀 Bootstrap remoto de conectividad DB en la VM...")
			if err := ensureDBBootstrapOnProvisionedVM(&cfg); err != nil {
				return fmt.Errorf("no se pudo bootstrapear conectividad DB remota: %w", err)
			}
			fmt.Println("🚀 Ejecutando migraciones iniciales del proyecto en la VM...")
			if err := runProjectMigrationsOnVM(cfg, defaultScaffoldMigrations, ""); err != nil {
				return fmt.Errorf("no se pudieron ejecutar migraciones iniciales en la VM: %w", err)
			}
		}

		if jsonOutput {
			safeResp := *resp
			safeResp.VMSshPrivateKey = ""
			safeResp.ProjectAPIToken = ""
			safeResp.DBPassword = ""
			if safeResp.Identity != nil {
				id := *safeResp.Identity
				id.ClientSecret = ""
				safeResp.Identity = &id
			}
			if safeResp.Auth != nil {
				a := *safeResp.Auth
				a.ClientSecret = ""
				safeResp.Auth = &a
			}
			if safeResp.MachineAuth != nil {
				m := *safeResp.MachineAuth
				m.ClientSecret = ""
				safeResp.MachineAuth = &m
			}
			if safeResp.AIGateway != nil {
				gw := *safeResp.AIGateway
				gw.APIKey = ""
				safeResp.AIGateway = &gw
			}
			if safeResp.IdentityExtensions != nil && safeResp.IdentityExtensions.CasdoorAdmin != nil {
				ca := *safeResp.IdentityExtensions.CasdoorAdmin
				ca.ClientSecret = ""
				safeResp.IdentityExtensions = &api.ProjectIdentityExtensions{CasdoorAdmin: &ca}
			}
			if safeResp.Secrets != nil {
				s := *safeResp.Secrets
				s.SSHPrivateKey = ""
				s.ProjectAPIToken = ""
				s.DBPassword = ""
				s.OIDCClientSecret = ""
				s.MachineClientSecret = ""
				s.CasdoorAdminClientSecret = ""
				s.AIGWAPIKey = ""
				safeResp.Secrets = &s
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(safeResp)
		}

		fmt.Println("✅ Proyecto creado")
		fmt.Printf("ID: %s\n", projectID)
		fmt.Printf("Name: %s\n", resp.Name)
		fmt.Printf("Slug: %s\n", slug)
		fmt.Printf("Subdomain: %s\n", resp.Subdomain)
		fmt.Printf("Status: %s\n", resp.Status)
		if wd, werr := os.Getwd(); werr == nil {
			fmt.Printf("Local folder: %s\n", wd)
		}
		if strings.TrimSpace(cfg.LastVMName) != "" {
			fmt.Printf("VM: %s\n", cfg.LastVMName)
		}
		if strings.TrimSpace(cfg.LastVMHTTPSURL) != "" {
			fmt.Printf("VM URL: %s\n", cfg.LastVMHTTPSURL)
		}
		if strings.TrimSpace(cfg.LastVMSshDest) != "" {
			fmt.Printf("VM SSH: %s\n", cfg.LastVMSshDest)
		}
		if strings.TrimSpace(cfg.MutagenDestination) != "" {
			fmt.Printf("Mutagen destination: %s\n", cfg.MutagenDestination)
		}
		if strings.TrimSpace(cfg.ProjectDBName) != "" {
			fmt.Println("DB credentials:")
			fmt.Printf("  dbName: %s\n", cfg.ProjectDBName)
			fmt.Printf("  dbUser: %s\n", cfg.ProjectDBUser)
			if strings.TrimSpace(cfg.ProjectDBPassword) != "" {
				fmt.Printf("  dbPassword: %s\n", config.MaskToken(cfg.ProjectDBPassword))
			} else {
				fmt.Printf("  dbPassword: %s\n", cfg.ProjectDBPassword)
			}
			fmt.Printf("  dbHost: %s\n", cfg.ProjectDBHost)
			if cfg.ProjectDBPort > 0 {
				fmt.Printf("  dbPort: %d\n", cfg.ProjectDBPort)
			}
		}
		if strings.TrimSpace(cfg.ProjectDatabaseURL) != "" {
			fmt.Printf("  databaseUrl: %s\n", maskDatabaseURL(cfg.ProjectDatabaseURL))
		}

		airStarted := false
		mutagenReady := false
		wedeReady := false
		httpChecked := false
		httpReady := false
		httpCode := 0
		var airErr error

		if !skipMutagenCheck {
			if err := ensureMutagenOnWindows(); err != nil {
				fmt.Printf("⚠️  Proyecto creado, pero no se pudo verificar/instalar Mutagen: %v\n", err)
				fmt.Println("   Puedes instalarlo manualmente y luego correr: einarc mutagen ...")
			} else {
				fmt.Println("✅ Mutagen disponible en esta máquina")
				if err := setupAndStartMutagen(&cfg); err != nil {
					fmt.Printf("⚠️  Mutagen disponible, pero no se pudo auto-configurar/start: %v\n", err)
					fmt.Printf("   Puedes correr manualmente: %s daemon start && %s project start\n", mutagenHintCommand(), mutagenHintCommand())
				} else {
					mutagenReady = true
					if !skipAirAutoStart {
						fmt.Println("🚀 Iniciando Air (mandatorio)...")
						maxAirAttempts := 6
						for i := 1; i <= maxAirAttempts; i++ {
							if i == 1 {
								fmt.Println("ℹ️  Preparando/verificando Air en VM...")
							}
							if err := setupAndStartRemoteAir(&cfg); err != nil {
								airErr = err
								errMsg := strings.ToLower(err.Error())
								if strings.Contains(errMsg, "downloading") || strings.Contains(errMsg, "timeout") {
									fmt.Printf("ℹ️  Air intento %d/%d: aún preparando dependencias (%v)\n", i, maxAirAttempts, err)
								} else {
									fmt.Printf("⚠️  Air intento %d/%d: %v\n", i, maxAirAttempts, err)
								}
								time.Sleep(time.Duration(i) * 2 * time.Second)
								continue
							}
							airStarted = true
							fmt.Println("✅ Air iniciado")
							break
						}
						if !airStarted {
							fmt.Println("❌ No se pudo iniciar Air automáticamente tras varios intentos")
							fmt.Println("   Revisa logs con: einarc dev logs -f")
						}
					}
				}
			}
		}

		fmt.Println("🧩 Instalando y levantando Wede en la VM provisionada...")
		if err := ensureRemoteWedeReady(&cfg); err != nil {
			return fmt.Errorf("init incompleto: no se pudo instalar/iniciar wede: %w", err)
		}
		wedeReady = true

		if strings.TrimSpace(cfg.LastVMHTTPSURL) != "" {
			httpChecked = true
			if airStarted {
				fmt.Printf("🌐 Verificando endpoint HTTP: %s\n", strings.TrimSpace(cfg.LastVMHTTPSURL))
				if code, err := waitForHTTPReady(strings.TrimSpace(cfg.LastVMHTTPSURL), 20*time.Second); err != nil {
					if code > 0 {
						httpCode = code
					}
					fmt.Printf("⚠️  HTTP aún no listo: %v\n", err)
					fmt.Println("   Puedes revisar con: einarc dev logs -f")
				} else {
					httpReady = true
					httpCode = code
					fmt.Printf("✅ App lista (%d): %s\n", code, strings.TrimSpace(cfg.LastVMHTTPSURL))
				}
			} else {
				fmt.Printf("⚠️  HTTP no verificable porque Air no inició: %s\n", strings.TrimSpace(cfg.LastVMHTTPSURL))
			}
		}

		fmt.Println("\n📌 Resumen final")
		if mutagenReady {
			fmt.Println("   - Sync: ✅")
		} else if skipMutagenCheck {
			fmt.Println("   - Sync: ⏭️  omitido (--skip-mutagen-check)")
		} else {
			fmt.Println("   - Sync: ⚠️")
		}
		if airStarted {
			fmt.Println("   - Air: ✅")
		} else if skipAirAutoStart {
			fmt.Println("   - Air: ⏭️  omitido (--skip-air-auto-start)")
		} else {
			fmt.Println("   - Air: ⚠️")
		}
		fmt.Println("   - Runtime del agente: ✅ dentro de cmd/api (sin sidecars)")
		if wedeReady {
			fmt.Println("   - Wede: ✅")
		} else {
			fmt.Println("   - Wede: ⚠️")
		}
		if !httpChecked {
			fmt.Println("   - HTTP: ⏭️  sin URL")
		} else if httpReady {
			fmt.Printf("   - HTTP: ✅ (%d)\n", httpCode)
		} else if httpCode > 0 {
			fmt.Printf("   - HTTP: ⚠️  (%d)\n", httpCode)
		} else {
			fmt.Println("   - HTTP: ⚠️")
		}
		if strings.TrimSpace(cfg.LastVMHTTPSURL) != "" {
			fmt.Printf("   - URL: %s\n", strings.TrimSpace(cfg.LastVMHTTPSURL))
		}
		if !airStarted || !httpReady {
			fmt.Println("   - Siguiente paso: einarc dev logs -f")
		}
		if !skipAirAutoStart && !airStarted {
			if airErr != nil {
				return fmt.Errorf("init incompleto: Air es mandatorio y no inició (%v)", airErr)
			}
			return fmt.Errorf("init incompleto: Air es mandatorio y no inició")
		}
		return nil
	},
}

func shouldRefreshAndRetry(err error, cfg *config.Config) (bool, error) {
	var ae api.APIError
	if !api.AsAPIError(err, &ae) || ae.StatusCode != 401 {
		return false, nil
	}
	ok, rerr := tryRefreshAndSave(cfg)
	if rerr != nil {
		return false, rerr
	}
	return ok, nil
}

func ensurePostgresProjectProvisioned(ctx context.Context, cfg *config.Config, projectName, machineClientID string) (*api.CreatePostgresProjectResponse, error) {
	name := strings.TrimSpace(projectName)
	if name == "" {
		return nil, fmt.Errorf("nombre de proyecto vacío para provisioning de Postgres API")
	}
	dbAPIURL := resolveDBAPIURL("")
	client := api.NewClient(dbAPIURL, cfg.Token, 15*time.Second)
	resp, err := client.CreatePostgresProject(ctx, name, machineClientID)
	if err != nil {
		if refreshed, rerr := shouldRefreshAndRetry(err, cfg); rerr != nil {
			return nil, rerr
		} else if refreshed {
			client = api.NewClient(dbAPIURL, cfg.Token, 15*time.Second)
			resp, err = client.CreatePostgresProject(ctx, name, machineClientID)
		}
	}
	if err != nil {
		var ae api.APIError
		if api.AsAPIError(err, &ae) && ae.StatusCode == 409 {
			fmt.Printf("ℹ️  Proyecto ya existe en Postgres API: %s\n", name)
			return nil, nil
		}
		if msg := mapAPIError(err); msg != "" {
			return nil, fmt.Errorf("falló provisioning en Postgres API (%s): %s", dbAPIURL, msg)
		}
		return nil, fmt.Errorf("falló provisioning en Postgres API (%s): %w", dbAPIURL, err)
	}

	fmt.Printf("✅ Proyecto provisionado en Postgres API (%s)\n", dbAPIURL)
	return resp, nil
}

func mapAPIError(err error) string {
	var ae api.APIError
	if !api.AsAPIError(err, &ae) {
		return ""
	}

	switch ae.StatusCode {
	case 401:
		return "Token inválido/revocado/expirado. Ejecuta login nuevamente."
	case 403:
		if strings.Contains(strings.ToLower(ae.Code), "scope") || strings.Contains(strings.ToLower(ae.Message), "scope") {
			return "Tu token no incluye scope projects:create."
		}
		return "No tienes permisos suficientes (owner/admin)."
	case 409:
		return "Ya existe un proyecto con ese nombre/slug en este tenant."
	case 422:
		return "El nombre no permite generar un slug válido. Intenta otro nombre."
	case 502:
		lower := strings.ToLower(strings.TrimSpace(ae.Message))
		if strings.Contains(lower, "vm name") && strings.Contains(lower, "not available") {
			return "El nombre de VM/slug no está disponible. Prueba con otro nombre de proyecto."
		}
		if strings.Contains(lower, "provisioner http failed") && strings.Contains(lower, "status=422") {
			return fmt.Sprintf("El provisionador rechazó la VM. Detalle: %s", ae.Message)
		}
		return fmt.Sprintf("Error API (%d): %s", ae.StatusCode, ae.Message)
	default:
		return fmt.Sprintf("Error API (%d): %s", ae.StatusCode, ae.Message)
	}
}

func buildDatabaseURL(user, password, host string, port int, dbName string) string {
	if strings.TrimSpace(user) == "" || strings.TrimSpace(password) == "" || strings.TrimSpace(host) == "" || strings.TrimSpace(dbName) == "" || port <= 0 {
		return ""
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", url.QueryEscape(user), url.QueryEscape(password), host, port, url.QueryEscape(dbName))
}

func extractEmailFromJWT(rawToken string) string {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return ""
	}
	parts := strings.Split(rawToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	for _, key := range []string{"email", "upn", "preferred_username", "unique_name", "name"} {
		if value, ok := claims[key].(string); ok && strings.Contains(strings.TrimSpace(value), "@") {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	for _, key := range []string{"emails", "email_addresses"} {
		if values, ok := claims[key].([]any); ok {
			for _, value := range values {
				if text, ok := value.(string); ok && strings.Contains(strings.TrimSpace(text), "@") {
					return strings.ToLower(strings.TrimSpace(text))
				}
			}
		}
	}
	return ""
}

func maskDatabaseURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.User == nil {
		return raw
	}
	username := u.User.Username()
	if username == "" {
		return raw
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		u.User = url.UserPassword(username, "***")
	}
	return u.String()
}

func ensureGoProjectScaffold(slug, bootstrapEmail string) error {
	s := strings.TrimSpace(slug)
	if s == "" {
		s = "app"
	}
	bootstrapEmail = strings.ToLower(strings.TrimSpace(bootstrapEmail))
	allowlistEntry := ""
	if bootstrapEmail != "" {
		allowlistEntry = fmt.Sprintf("\t%q: {},\n", bootstrapEmail)
	}
	if err := scaffold.MaterializeAppMobileDownloader(".", s, bootstrapEmail); err != nil {
		return err
	}
	fmt.Println("✅ scaffold app-mobile-downloader generado")
	if strings.TrimSpace(os.Getenv("EINAR_USE_LEGACY_INLINE_SCAFFOLD")) != "true" {
		return nil
	}

	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		goMod := fmt.Sprintf("module %s\n\ngo 1.22\n", s)
		if werr := os.WriteFile("go.mod", []byte(goMod), 0o644); werr != nil {
			return werr
		}
		fmt.Println("✅ go.mod generado")
	}

	if _, err := os.Stat("cmd/api/main.go"); os.IsNotExist(err) {
		if _, rootErr := os.Stat("main.go"); rootErr == nil {
			fmt.Println("ℹ️  main.go ya existe en raíz, se respeta (no se genera cmd/api/main.go)")
		} else {
			if mkErr := os.MkdirAll("cmd/api", 0o755); mkErr != nil {
				return mkErr
			}
			mainGo := fmt.Sprintf(`package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	_ %q
	_ %q
	_ %q
	_ %q

	"github.com/Ignaciojeria/ioc"
)

func main() {
	if err := ioc.LoadDependencies(); err != nil {
		log.Fatal(err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	if err := ioc.Shutdown(); err != nil {
		log.Fatalf("Shutdown errors: %%v", err)
	}
}
`, s+"/internal/shared/jwks", s+"/internal/shared/server", s+"/internal/adapter/in/web", s+"/internal/shared/infrastructure/postgresql")
			if werr := os.WriteFile("cmd/api/main.go", []byte(mainGo), 0o644); werr != nil {
				return werr
			}
			fmt.Println("✅ cmd/api/main.go generado")
		}
	}

	sharedFiles := map[string]string{
		"internal/shared/configuration/conf.go": `package configuration

import (
	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewConf)

type Conf struct {
	PORT         string ` + "`env:\"PORT\" envDefault:\"8000\"`" + `
	PROJECT_NAME string ` + "`env:\"PROJECT_NAME\"`" + `
	VERSION      string ` + "`env:\"VERSION\"`" + `

	DATABASE_URL string ` + "`env:\"DATABASE_URL\" envDefault:\"postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable\"`" + `

	OIDCType                 string ` + "`env:\"OIDC_TYPE\" envDefault:\"oidc\"`" + `
	OIDCProvider             string ` + "`env:\"OIDC_PROVIDER\" envDefault:\"casdoor\"`" + `
	OIDCIssuer               string ` + "`env:\"OIDC_ISSUER\"`" + `
	OIDCDiscoveryURL         string ` + "`env:\"OIDC_DISCOVERY_URL\"`" + `
	OIDCJWKSURI              string ` + "`env:\"OIDC_JWKS_URI\"`" + `
	OIDCAuthorizationEndpoint string ` + "`env:\"OIDC_AUTHORIZATION_ENDPOINT\"`" + `
	OIDCTokenEndpoint        string ` + "`env:\"OIDC_TOKEN_ENDPOINT\"`" + `
	OIDCUserinfoEndpoint     string ` + "`env:\"OIDC_USERINFO_ENDPOINT\"`" + `
	OIDCClientID             string ` + "`env:\"OIDC_CLIENT_ID\"`" + `
	OIDCClientSecret         string ` + "`env:\"OIDC_CLIENT_SECRET\"`" + `
	OIDCClientSecretRef      string ` + "`env:\"OIDC_CLIENT_SECRET_REF\"`" + `
	OIDCRedirectURI          string ` + "`env:\"OIDC_REDIRECT_URI\"`" + `
	OIDCLogoutURI            string ` + "`env:\"OIDC_LOGOUT_URI\"`" + `
	OIDCPostLogoutRedirectURI string ` + "`env:\"OIDC_POST_LOGOUT_REDIRECT_URI\"`" + `
	OIDCScopes               string ` + "`env:\"OIDC_SCOPES\" envDefault:\"openid profile email\"`" + `
	OIDCLoginURL             string ` + "`env:\"OIDC_LOGIN_URL\"`" + `
	OIDCGoogleLoginURL       string ` + "`env:\"OIDC_GOOGLE_LOGIN_URL\"`" + `
	OIDCUpstreamGoogleClientID string ` + "`env:\"OIDC_UPSTREAM_GOOGLE_CLIENT_ID\"`" + `

	MachineAuthGrantType      string ` + "`env:\"MACHINE_AUTH_GRANT_TYPE\" envDefault:\"client_credentials\"`" + `
	MachineAuthTokenEndpoint  string ` + "`env:\"MACHINE_AUTH_TOKEN_ENDPOINT\"`" + `
	MachineAuthClientID       string ` + "`env:\"MACHINE_AUTH_CLIENT_ID\"`" + `
	MachineAuthClientSecret   string ` + "`env:\"MACHINE_AUTH_CLIENT_SECRET\"`" + `
	MachineAuthClientSecretRef string ` + "`env:\"MACHINE_AUTH_CLIENT_SECRET_REF\"`" + `
	MachineAuthAudience       string ` + "`env:\"MACHINE_AUTH_AUDIENCE\"`" + `
	MachineAuthScopes         string ` + "`env:\"MACHINE_AUTH_SCOPES\"`" + `

	AUTH_DISABLED string ` + "`env:\"AUTH_DISABLED\" envDefault:\"false\"`" + `
	JWKSURLS      string ` + "`env:\"JWKS_URLS\"`" + `
	JWTAudience   string ` + "`env:\"JWT_AUDIENCE\"`" + `
}

func NewConf() (Conf, error) {
	return Parse[Conf]()
}

func (c Conf) GetOIDCIssuer() string { return c.OIDCIssuer }
func (c Conf) GetOIDCClientID() string { return c.OIDCClientID }
func (c Conf) GetOIDCTokenEndpoint() string { return c.OIDCTokenEndpoint }
func (c Conf) GetOIDCClientSecret() string { return c.OIDCClientSecret }
func (c Conf) GetJWTAudience() string { return c.JWTAudience }
`,
		"internal/shared/configuration/parse.go": `package configuration

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

var once sync.Once

func handleEnvLoad(err error) {
	if err != nil {
		slog.Warn(".env not found, loading environment variables from system.")
	} else {
		slog.Info("Environment variables loaded from .env file.")
	}
}

func findProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for dir != filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return wd
}

func loadEnvOnce() {
	once.Do(func() {
		root := findProjectRoot()
		envPath := filepath.Join(root, ".env")
		handleEnvLoad(godotenv.Load(envPath))
	})
}

func Parse[T any]() (T, error) {
	loadEnvOnce()
	var conf T
	if err := env.Parse(&conf); err != nil {
		return conf, fmt.Errorf("failed to parse configuration: %w", err)
	}
	return conf, nil
}
`,
		"internal/shared/infrastructure/postgresql/connection.go": fmt.Sprintf(`package postgresql

import (
	"embed"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	%q

	"github.com/Ignaciojeria/ioc"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

var _ = ioc.Register(NewConnection)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func NewConnection(conf configuration.Conf) (*sqlx.DB, error) {
	dsn := strings.TrimSpace(conf.DATABASE_URL)
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %%w", err)
	}

	u, err := url.Parse(dsn)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("invalid DATABASE_URL format: %%w", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if err := runMigrations(db, dbName); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %%w", err)
	}
	return db, nil
}

func runMigrations(db *sqlx.DB, dbName string) error {
	if db == nil {
		return fmt.Errorf("db connection is nil")
	}
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	driver, err := postgres.WithInstance(db.DB, &postgres.Config{DatabaseName: dbName})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", d, dbName, driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	slog.Info("Database migrations validated/applied successfully")
	return nil
}
`, s+"/internal/shared/configuration"),
		"internal/shared/infrastructure/postgresql/migrations/000001_create_users_and_sessions.up.sql": `CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject TEXT NOT NULL UNIQUE,
    email TEXT,
    display_name TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_token TEXT,
    refresh_token TEXT,
    id_token TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
`,
		"internal/shared/infrastructure/postgresql/migrations/000001_create_users_and_sessions.down.sql": `DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
`,
		"internal/shared/jwks/new.go": fmt.Sprintf(`package jwks

import (
	"strings"

	%q

	"github.com/Ignaciojeria/ioc"
	"github.com/MicahParks/keyfunc/v3"
)

var _ = ioc.Register(New)

func New(conf configuration.Conf) (keyfunc.Keyfunc, error) {
	urlsValue := strings.TrimSpace(conf.JWKSURLS)
	if urlsValue == "" {
		urlsValue = strings.TrimSpace(conf.OIDCJWKSURI)
	}
	urls := strings.Split(urlsValue, ",")
	cleaned := make([]string, 0, len(urls))
	for i := range urls {
		if value := strings.TrimSpace(urls[i]); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return keyfunc.NewDefault(cleaned)
}
`, s+"/internal/shared/configuration"),
		"internal/shared/server/new.go": fmt.Sprintf(`package server

import (
	"context"
	"strings"
	"time"

	%q
	%q

	"github.com/Ignaciojeria/ioc"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-fuego/fuego"
	"github.com/jmoiron/sqlx"
)

var _ = ioc.Register(New)
var _ = ioc.Register(startServer)

type Server struct {
	*fuego.Server
}

func New(conf configuration.Conf, jwks keyfunc.Keyfunc, db *sqlx.DB) *Server {
	server := fuego.NewServer(fuego.WithAddr(":" + strings.TrimSpace(conf.PORT)))
	fuego.Use(server, middleware.JWTMiddleware(
		jwks,
		db,
		conf,
	))
	return &Server{Server: server}
}

func startServer(
	server *Server,
	shutdowner ioc.Shutdowner,
) error {
	go func() {
		if err := server.Run(); err != nil {
			panic(err)
		}
	}()

	shutdowner.RegisterShutdown(func() error {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		return server.Shutdown(ctx)
	})

	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
`, s+"/internal/shared/configuration", s+"/internal/shared/server/middleware"),
		"internal/shared/server/middleware/middleware.go": fmt.Sprintf(`package middleware

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	%q

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
)

type contextKey string

const (
	claimsContextKey          contextKey = "jwt_claims"
	sessionCookieName                    = "app_session_id"
)

type oidcConfiguration interface {
	GetOIDCIssuer() string
	GetOIDCClientID() string
	GetOIDCTokenEndpoint() string
	GetOIDCClientSecret() string
	GetJWTAudience() string
}

func JWTMiddleware(
	jwks keyfunc.Keyfunc,
	db *sqlx.DB,
	conf oidcConfiguration,
) func(http.Handler) http.Handler {
	issuer := ""
	audience := ""
	if conf != nil {
		issuer = strings.TrimSpace(conf.GetOIDCIssuer())
		audience = firstNonEmpty(strings.TrimSpace(conf.GetJWTAudience()), strings.TrimSpace(conf.GetOIDCClientID()))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if os.Getenv("AUTH_DISABLED") == "true" {
				sub := strings.TrimSpace(r.Header.Get("X-Dev-Sub"))
				if sub == "" {
					sub = "dev-user"
				}
				ctx := context.WithValue(r.Context(), claimsContextKey, jwt.MapClaims{"sub": sub})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if db != nil {
				if claims, ok := claimsFromSessionCookie(r, w, db, conf); ok {
					if !isAuthorizedPathClaims(r.URL.Path, claims) {
						writeForbidden(w)
						return
					}
					ctx := context.WithValue(r.Context(), claimsContextKey, claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				if shouldRedirectToLogin(r) {
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}
				writeUnauthorized(w)
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := parseJWTClaims(jwks, tokenString, issuer, audience)
			if err != nil {
				if shouldRedirectToLogin(r) {
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}
				writeUnauthorized(w)
				return
			}
			if !isAuthorizedPathClaims(r.URL.Path, claims) {
				writeForbidden(w)
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type Principal struct {
	Subject         string
	MachineClientID string
	IsMachine       bool
}

type sessionRecord struct {
	ID           string
	UserID       string
	Subject      string
	Email        sql.NullString
	DisplayName  sql.NullString
	AccessToken  sql.NullString
	RefreshToken sql.NullString
	IDToken      sql.NullString
	ExpiresAt    sql.NullTime
}

func JWTClaimsFromContext(ctx context.Context) (jwt.MapClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(jwt.MapClaims)
	return claims, ok
}

func PrincipalFromClaims(claims jwt.MapClaims) Principal {
	subject, _ := claims["sub"].(string)
	subject = strings.TrimSpace(subject)
	grantType := normalizeGrantType(firstStringClaim(claims, "gty", "grant_type"))
	tokenUse := strings.ToLower(strings.TrimSpace(firstStringClaim(claims, "token_use", "type")))
	isMachine := grantType == "client_credentials" || tokenUse == "machine" || tokenUse == "application"

	machineClientID := firstStringClaim(claims, "client_id", "azp", "cid")
	if strings.TrimSpace(machineClientID) == "" && isMachine {
		machineClientID = firstAudienceClaim(claims)
	}
	if strings.TrimSpace(machineClientID) == "" {
		machineClientID = firstNonEmptyMachineID(subject, firstStringClaim(claims, "name", "id"))
	}
	if subject == "" && strings.TrimSpace(machineClientID) != "" {
		isMachine = true
	}
	if tokenUse == "application" && strings.TrimSpace(machineClientID) != "" {
		isMachine = true
	}
	return Principal{
		Subject:         subject,
		MachineClientID: strings.TrimSpace(machineClientID),
		IsMachine:       isMachine,
	}
}

func claimsFromSessionCookie(r *http.Request, w http.ResponseWriter, db *sqlx.DB, conf oidcConfiguration) (jwt.MapClaims, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, false
	}

	var rec sessionRecord
	query := "SELECT s.id, s.user_id, s.access_token, s.refresh_token, s.id_token, s.expires_at, u.subject, u.email, u.display_name " +
		"FROM sessions s JOIN users u ON u.id = s.user_id " +
		"WHERE s.id = $1 AND s.revoked_at IS NULL"
	row := db.QueryRowx(query, strings.TrimSpace(cookie.Value))
	if err := row.Scan(&rec.ID, &rec.UserID, &rec.AccessToken, &rec.RefreshToken, &rec.IDToken, &rec.ExpiresAt, &rec.Subject, &rec.Email, &rec.DisplayName); err != nil {
		clearSessionCookie(w)
		return nil, false
	}

	if rec.ExpiresAt.Valid && time.Now().After(rec.ExpiresAt.Time) {
		if conf == nil || strings.TrimSpace(conf.GetOIDCTokenEndpoint()) == "" || !rec.RefreshToken.Valid || strings.TrimSpace(rec.RefreshToken.String) == "" {
			clearSessionCookie(w)
			return nil, false
		}
		if err := refreshSessionTokens(db, conf, &rec); err != nil {
			clearSessionCookie(w)
			return nil, false
		}
	}

	claims := jwt.MapClaims{
		"sub": rec.Subject,
		"sid": rec.ID,
	}
	if rec.Email.Valid && strings.TrimSpace(rec.Email.String) != "" {
		claims["email"] = strings.TrimSpace(rec.Email.String)
	}
	if rec.DisplayName.Valid && strings.TrimSpace(rec.DisplayName.String) != "" {
		claims["name"] = strings.TrimSpace(rec.DisplayName.String)
	}
	return claims, true
}

func refreshSessionTokens(db *sqlx.DB, conf oidcConfiguration, rec *sessionRecord) error {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(rec.RefreshToken.String))
	form.Set("client_id", strings.TrimSpace(conf.GetOIDCClientID()))
	if secret := strings.TrimSpace(conf.GetOIDCClientSecret()); secret != "" {
		form.Set("client_secret", secret)
	}

	resp, err := http.PostForm(strings.TrimSpace(conf.GetOIDCTokenEndpoint()), form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("refresh token exchange failed with status %%d", resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	accessToken, _ := out["access_token"].(string)
	if strings.TrimSpace(accessToken) == "" {
		return fmt.Errorf("refresh token exchange returned empty access token")
	}
	refreshToken, _ := out["refresh_token"].(string)
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(rec.RefreshToken.String)
	}
	idToken, _ := out["id_token"].(string)
	idToken = strings.TrimSpace(idToken)
	if idToken == "" && rec.IDToken.Valid {
		idToken = strings.TrimSpace(rec.IDToken.String)
	}
	var expiresAt any
	if expiresIn, ok := out["expires_in"].(float64); ok && int(expiresIn) > 0 {
		expiresAt = time.Now().Add(time.Duration(int(expiresIn)) * time.Second)
	}
	if _, err := db.Exec("UPDATE sessions SET access_token=$1, refresh_token=$2, id_token=$3, expires_at=$4, updated_at=NOW() WHERE id=$5", strings.TrimSpace(accessToken), refreshToken, idToken, expiresAt, rec.ID); err != nil {
		return err
	}

	rec.AccessToken = sql.NullString{String: strings.TrimSpace(accessToken), Valid: true}
	rec.RefreshToken = sql.NullString{String: refreshToken, Valid: refreshToken != ""}
	rec.IDToken = sql.NullString{String: idToken, Valid: idToken != ""}
	if ts, ok := expiresAt.(time.Time); ok {
		rec.ExpiresAt = sql.NullTime{Time: ts, Valid: true}
	}
	return nil
}

func parseJWTClaims(jwks keyfunc.Keyfunc, tokenString, issuer, audience string) (jwt.MapClaims, error) {
	opts := []jwt.ParserOption{}
	if issuer != "" {
		opts = append(opts, jwt.WithIssuer(issuer))
	}
	if audience != "" {
		opts = append(opts, jwt.WithAudience(audience))
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, jwks.Keyfunc, opts...)
	if err != nil || !token.Valid {
		if err == nil {
			err = fmt.Errorf("invalid token")
		}
		return nil, err
	}
	return claims, nil
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func normalizeGrantType(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.ReplaceAll(v, "-", "_")
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyMachineID(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstAudienceClaim(claims jwt.MapClaims) string {
	raw, ok := claims["aud"]
	if !ok || raw == nil {
		return ""
	}
	switch aud := raw.(type) {
	case string:
		return strings.TrimSpace(aud)
	case []string:
		for _, value := range aud {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	case []any:
		for _, value := range aud {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstStringClaim(claims jwt.MapClaims, keys ...string) string {
	for _, key := range keys {
		if value, ok := claims[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isAuthorizedPathClaims(path string, claims jwt.MapClaims) bool {
	email := firstStringClaim(claims, "email")
	if strings.TrimSpace(email) == "" {
		return true
	}
	if isEditorPath(path) {
		return access.IsAllowedEditorEmail(email)
	}
	return access.IsAllowedAppEmail(email)
}

func shouldRedirectToLogin(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	accept := strings.ToLower(strings.TrimSpace(r.Header.Get("Accept")))
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml")
}

func isEditorPath(path string) bool {
	path = strings.TrimSpace(path)
	switch path {
	case "/editor", "/api":
		return true
	default:
		return strings.HasPrefix(path, "/editor/") ||
			strings.HasPrefix(path, "/assets/") ||
			strings.HasPrefix(path, "/api/")
	}
}

func isPublicPath(path string) bool {
	switch strings.TrimSpace(path) {
	case "/auth/login", "/auth/login/google", "/auth/callback", "/auth/logout", "/manifest.json", "/favicon.ico", "/icon.svg", "/icon-180.png":
		return true
	default:
		return false
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "unauthorized",
	})
}

func writeForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "forbidden",
	})
}
`, s+"/internal/shared/access"),
		"internal/shared/access/allowlist.go": fmt.Sprintf(`package access

import "strings"

func IsAllowedAppEmail(email string) bool {
	email = normalizeEmail(email)
	if email == "" {
		return false
	}
	_, ok := allowedAppEmails[email]
	return ok
}

func IsAllowedEditorEmail(email string) bool {
	email = normalizeEmail(email)
	if email == "" {
		return false
	}
	_, ok := allowedEditorEmails[email]
	return ok
}

func IsAllowedAnyEmail(email string) bool {
	return IsAllowedAppEmail(email) || IsAllowedEditorEmail(email)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

var allowedAppEmails = map[string]struct{}{
%s}

var allowedEditorEmails = map[string]struct{}{
%s}
`, allowlistEntry, allowlistEntry),
		"internal/adapter/in/web/hello.go": fmt.Sprintf(`package in

import (
	%q

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(helloWorldHandler)

func helloWorldHandler(s *server.Server) {
	fuego.All(s.Server, "/", func(c fuego.ContextNoBody) (string, error) {
		return "hello world!", nil
	})
}
`, s+"/internal/shared/server"),
		"internal/adapter/in/web/wede.go": fmt.Sprintf(`package in

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	%q

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(wedeHandler)

func wedeHandler(s *server.Server) {
	upstream := strings.TrimSpace(os.Getenv("WEDE_UPSTREAM_URL"))
	if upstream == "" {
		upstream = "http://127.0.0.1:9090"
	}

	target, err := url.Parse(upstream)
	if err != nil {
		log.Printf("invalid WEDE_UPSTREAM_URL %%q: %%v", upstream, err)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.Host = target.Host
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/editor")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		r.URL.RawPath = r.URL.Path
		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
		r.Header.Set("X-Forwarded-Proto", forwardedProto(r))
		if prefix := strings.TrimSpace(r.Header.Get("X-Forwarded-Prefix")); prefix == "" {
			r.Header.Set("X-Forwarded-Prefix", "/editor")
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("wede proxy error: %%v", err)
		http.Error(w, "wede upstream unavailable", http.StatusBadGateway)
	}

	for _, path := range []string{
		"/editor", "/editor/",
		"/assets/",
		"/api/", "/api",
		"/manifest.json",
		"/favicon.ico",
		"/icon.svg",
		"/icon-180.png",
	} {
		fuego.Handle(s.Server, path, proxy)
	}
}

func forwardedProto(r *http.Request) string {
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
`, s+"/internal/shared/server"),
		"internal/adapter/in/web/auth_login.go": fmt.Sprintf(`package in

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	%q
	%q

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(authLoginHandler)

func authLoginHandler(s *server.Server, conf configuration.Conf) {
	handleLogin := func(c fuego.ContextNoBody, preferGoogle bool) (any, error) {
		state, err := randomState()
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "cannot generate oauth state"}
		}

		http.SetCookie(c.Response(), &http.Cookie{
			Name:     "oidc_state",
			Value:    state,
			Path:     "/",
			HttpOnly: true,
			Secure:   isHTTPS(conf.OIDCRedirectURI),
			SameSite: http.SameSiteLaxMode,
		})

		if preferGoogle && strings.TrimSpace(conf.OIDCUpstreamGoogleClientID) != "" {
			googleURL, err := buildDirectGoogleLoginURL(conf, state)
			if err != nil {
				return nil, fuego.HTTPError{Status: http.StatusBadGateway, Title: "could not build direct google redirect", Detail: err.Error()}
			}
			http.Redirect(c.Response(), c.Request(), googleURL, http.StatusFound)
			return nil, nil
		}

		loginURL, err := buildLoginURL(conf, state, preferGoogle)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		http.Redirect(c.Response(), c.Request(), loginURL, http.StatusFound)
		return nil, nil
	}

	fuego.Get(s.Server, "/auth/login", func(c fuego.ContextNoBody) (any, error) {
		return handleLogin(c, false)
	})
	fuego.Get(s.Server, "/auth/login/google", func(c fuego.ContextNoBody) (any, error) {
		return handleLogin(c, true)
	})
}

func buildLoginURL(conf configuration.Conf, state string, preferGoogle bool) (string, error) {
	base := strings.TrimSpace(conf.OIDCLoginURL)
	if preferGoogle && strings.TrimSpace(conf.OIDCGoogleLoginURL) != "" {
		base = strings.TrimSpace(conf.OIDCGoogleLoginURL)
	}
	if base == "" {
		base = strings.TrimSpace(conf.OIDCAuthorizationEndpoint)
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", strings.TrimSpace(conf.OIDCClientID))
	q.Set("redirect_uri", strings.TrimSpace(conf.OIDCRedirectURI))
	q.Set("response_type", "code")
	q.Set("scope", firstNonEmptyScope(strings.TrimSpace(conf.OIDCScopes), "openid profile email"))
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func buildDirectGoogleLoginURL(conf configuration.Conf, state string) (string, error) {
	googleClientID := strings.TrimSpace(conf.OIDCUpstreamGoogleClientID)
	if googleClientID == "" {
		return "", fmt.Errorf("OIDC_UPSTREAM_GOOGLE_CLIENT_ID is empty")
	}
	issuer := strings.TrimRight(strings.TrimSpace(conf.OIDCIssuer), "/")
	if issuer == "" {
		return "", fmt.Errorf("OIDC_ISSUER is empty")
	}
	appName, err := deriveCasdoorAppName(conf.OIDCClientID, conf.PROJECT_NAME)
	if err != nil {
		return "", err
	}

	scope := firstNonEmptyScope(strings.TrimSpace(conf.OIDCScopes), "openid profile email")
	packedQ := url.Values{}
	packedQ.Set("client_id", strings.TrimSpace(conf.OIDCClientID))
	packedQ.Set("redirect_uri", strings.TrimSpace(conf.OIDCRedirectURI))
	packedQ.Set("response_type", "code")
	packedQ.Set("scope", scope)
	packedQ.Set("state", state)

	packed := "?" + packedQ.Encode() +
		"&application=" + url.QueryEscape(appName) +
		"&provider=" + url.QueryEscape("provider_google_einar") +
		"&method=" + url.QueryEscape("signup")
	packedState := base64.StdEncoding.EncodeToString([]byte(packed))

	googleQ := url.Values{}
	googleQ.Set("client_id", googleClientID)
	googleQ.Set("redirect_uri", issuer+"/callback")
	googleQ.Set("scope", "openid email profile")
	googleQ.Set("response_type", "code")
	googleQ.Set("state", packedState)
	return "https://accounts.google.com/signin/oauth?" + googleQ.Encode(), nil
}

func deriveCasdoorAppName(clientID, projectSlug string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	projectSlug = strings.TrimSpace(projectSlug)
	if clientID == "" || projectSlug == "" {
		return "", fmt.Errorf("cannot derive app name: empty client id or project slug")
	}
	needle := "-" + projectSlug + "-"
	idx := strings.Index(clientID, needle)
	if idx < 0 {
		return "", fmt.Errorf("cannot derive app name from client id")
	}
	return clientID[:idx+len(needle)-1], nil
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isHTTPS(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https")
}

func firstNonEmptyScope(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
`, s+"/internal/shared/configuration", s+"/internal/shared/server"),
		"internal/adapter/in/web/auth_callback.go": fmt.Sprintf(`package in

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	%q
	%q
	%q

	"github.com/Ignaciojeria/ioc"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-fuego/fuego"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
)

var _ = ioc.Register(authCallbackHandler)

type authCallbackResponse struct {
	AccessToken  string `+"`json:\"access_token,omitempty\"`"+`
	RefreshToken string `+"`json:\"refresh_token,omitempty\"`"+`
	IDToken      string `+"`json:\"id_token,omitempty\"`"+`
	TokenType    string `+"`json:\"token_type,omitempty\"`"+`
	ExpiresIn    int    `+"`json:\"expires_in,omitempty\"`"+`
}

type oidcIdentity struct {
	Subject     string
	Email       string
	DisplayName string
}

func authCallbackHandler(s *server.Server, conf configuration.Conf, db *sqlx.DB, jwks keyfunc.Keyfunc) {
	fuego.Get(s.Server, "/auth/callback", func(c fuego.ContextNoBody) (any, error) {
		state := strings.TrimSpace(c.QueryParam("state"))
		code := strings.TrimSpace(c.QueryParam("code"))
		if code == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "missing code"}
		}

		stateCookie, err := c.Request().Cookie("oidc_state")
		if err != nil || strings.TrimSpace(stateCookie.Value) == "" || stateCookie.Value != state {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "invalid oauth state"}
		}

		resp, err := exchangeAuthorizationCode(conf, code)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		identity, err := extractIdentityFromTokens(conf, jwks, resp)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		if !access.IsAllowedAnyEmail(identity.Email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "email sin acceso autorizado al sistema"}
		}
		sessionID, err := persistUserSession(db, identity, resp)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		http.SetCookie(c.Response(), &http.Cookie{
			Name:     "oidc_state",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   isHTTPS(conf.OIDCRedirectURI),
			SameSite: http.SameSiteLaxMode,
		})
		http.SetCookie(c.Response(), &http.Cookie{
			Name:     "app_session_id",
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   isHTTPS(conf.OIDCRedirectURI),
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(c.Response(), c.Request(), "/", http.StatusFound)
		return nil, nil
	})

	fuego.Get(s.Server, "/auth/logout", func(c fuego.ContextNoBody) (any, error) {
		if cookie, err := c.Request().Cookie("app_session_id"); err == nil && strings.TrimSpace(cookie.Value) != "" {
			_, _ = db.Exec("UPDATE sessions SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1", strings.TrimSpace(cookie.Value))
		}
		http.SetCookie(c.Response(), &http.Cookie{Name: "app_session_id", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(conf.OIDCRedirectURI), SameSite: http.SameSiteLaxMode})
		http.Redirect(c.Response(), c.Request(), "/", http.StatusFound)
		return nil, nil
	})
}

func exchangeAuthorizationCode(conf configuration.Conf, code string) (authCallbackResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", strings.TrimSpace(conf.OIDCRedirectURI))
	form.Set("client_id", strings.TrimSpace(conf.OIDCClientID))
	if secret := strings.TrimSpace(conf.OIDCClientSecret); secret != "" {
		form.Set("client_secret", secret)
	}

	resp, err := http.PostForm(strings.TrimSpace(conf.OIDCTokenEndpoint), form)
	if err != nil {
		return authCallbackResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authCallbackResponse{}, fmt.Errorf("token exchange failed with status %%d", resp.StatusCode)
	}

	var out authCallbackResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return authCallbackResponse{}, err
	}
	return out, nil
}

func extractIdentityFromTokens(conf configuration.Conf, jwks keyfunc.Keyfunc, resp authCallbackResponse) (oidcIdentity, error) {
	issuer := strings.TrimSpace(conf.OIDCIssuer)
	audience := firstNonEmpty(strings.TrimSpace(conf.JWTAudience), strings.TrimSpace(conf.OIDCClientID))
	for _, tokenString := range []string{strings.TrimSpace(resp.IDToken), strings.TrimSpace(resp.AccessToken)} {
		if tokenString == "" {
			continue
		}
		claims := jwt.MapClaims{}
		opts := []jwt.ParserOption{}
		if issuer != "" {
			opts = append(opts, jwt.WithIssuer(issuer))
		}
		if audience != "" {
			opts = append(opts, jwt.WithAudience(audience))
		}
		token, err := jwt.ParseWithClaims(tokenString, claims, jwks.Keyfunc, opts...)
		if err != nil || !token.Valid {
			continue
		}
		identity := oidcIdentity{
			Subject:     strings.TrimSpace(firstStringClaim(claims, "sub")),
			Email:       strings.TrimSpace(firstStringClaim(claims, "email", "name")),
			DisplayName: strings.TrimSpace(firstStringClaim(claims, "display_name", "name", "email")),
		}
		if identity.Subject != "" {
			return identity, nil
		}
	}
	return oidcIdentity{}, fmt.Errorf("could not extract authenticated subject from oidc tokens")
}

func persistUserSession(db *sqlx.DB, identity oidcIdentity, resp authCallbackResponse) (string, error) {
	if db == nil {
		return "", fmt.Errorf("db connection is nil")
	}
	if strings.TrimSpace(identity.Subject) == "" {
		return "", fmt.Errorf("oidc subject is empty")
	}

	tx, err := db.Beginx()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var userID string
	userSQL := "INSERT INTO users (subject, email, display_name, created_at, updated_at) " +
		"VALUES ($1, $2, $3, NOW(), NOW()) " +
		"ON CONFLICT (subject) DO UPDATE SET email = EXCLUDED.email, display_name = EXCLUDED.display_name, updated_at = NOW() " +
		"RETURNING id"
	if err := tx.Get(&userID, userSQL, identity.Subject, nullableString(identity.Email), nullableString(identity.DisplayName)); err != nil {
		return "", err
	}

	var sessionID string
	var expiresAt any
	if resp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	}
	sessionSQL := "INSERT INTO sessions (user_id, access_token, refresh_token, id_token, expires_at, created_at, updated_at) " +
		"VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) RETURNING id"
	if err := tx.Get(&sessionID, sessionSQL, userID, nullableString(resp.AccessToken), nullableString(resp.RefreshToken), nullableString(resp.IDToken), expiresAt); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return sessionID, nil
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func firstStringClaim(claims jwt.MapClaims, keys ...string) string {
	for _, key := range keys {
		if value, ok := claims[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
`, s+"/internal/shared/access", s+"/internal/shared/configuration", s+"/internal/shared/server"),
	}

	for path, content := range sharedFiles {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
				return mkErr
			}
			if werr := os.WriteFile(path, []byte(content), 0o644); werr != nil {
				return werr
			}
			fmt.Printf("✅ %s generado\n", path)
		}
	}

	return nil
}

func airTomlContent() string {
	return `root = "."

tmp_dir = "tmp"

[build]
	cmd = "sh -c 'TEMPL_BIN=$(command -v templ || true); if [ -z \"$TEMPL_BIN\" ] && [ -x \"$HOME/go/bin/templ\" ]; then TEMPL_BIN=\"$HOME/go/bin/templ\"; fi; if [ -z \"$TEMPL_BIN\" ]; then echo \"templ not found; run: go install github.com/a-h/templ/cmd/templ@latest\"; exit 127; fi; \"$TEMPL_BIN\" generate && if [ -f ./cmd/api/main.go ]; then go build -o ./tmp/main ./cmd/api; elif [ -f ./cmd/main.go ]; then go build -o ./tmp/main ./cmd; else go build -o ./tmp/main .; fi'"
  bin = "./tmp/main"
	include_ext = ["go", "templ", "tpl", "tmpl", "html"]
	exclude_ext = ["_templ.go"]
  exclude_dir = ["assets", "tmp", "vendor", "node_modules", ".git", ".einar"]
  delay = 500

[log]
  time = true
`
}

func ensureLocalAirConfig() error {
	if _, err := os.Stat(".air.toml"); err == nil {
		return nil
	}
	if err := os.WriteFile(".air.toml", []byte(airTomlContent()), 0o644); err != nil {
		return err
	}
	fmt.Println("✅ .air.toml generado")
	return nil
}

func workspaceYAMLContent(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = "unknown-project"
	}
	return fmt.Sprintf(`version: 1

project:
  slug: %s

defaults:
  baseBranch: main
  portStart: 3000
  portStrategy: sequential # sequential | hash
  runtime: air
  sync: mutagen
  previewDomain: project.dev

templates:
  prod:
    branch: main
    port: 3000
    persistent: true
    runtime: air

  develop:
    branch: develop
    port: 3001
    persistent: true
    runtime: air

rules:
  - pattern: "feature/*"
    persistent: false
    runtime: air

  - pattern: "issue/*"
    persistent: false
    runtime: air

  - pattern: "agent/*"
    persistent: false
    runtime: air
`, slug)
}

func ensureWorkspaceConfig(slug string) error {
	if _, err := os.Stat("workspaces.yaml"); err == nil {
		return nil
	}
	if err := os.WriteFile("workspaces.yaml", []byte(workspaceYAMLContent(slug)), 0o644); err != nil {
		return err
	}
	fmt.Println("✅ workspaces.yaml generado")
	return nil
}

func ensureProjectGitIgnore() error {
	const path = ".gitignore"
	required := []string{
		".einar/",
		".env",
		"tmp/",
		".air.log",
		".air.pid",
		"mutagen.yml.lock",
	}

	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return err
	}

	toAdd := make([]string, 0, len(required))
	for _, rule := range required {
		if !strings.Contains(existing, rule) {
			toAdd = append(toAdd, rule)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(existing)
	if strings.TrimSpace(existing) != "" && !strings.HasSuffix(existing, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("\n# Einar local runtime files\n")
	for _, rule := range toAdd {
		sb.WriteString(rule)
		sb.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("✅ .gitignore actualizado")
	return nil
}

func ensureGitHooksForWorkspaceLock() error {
	if !isGitRepository() {
		return nil
	}
	if err := os.MkdirAll(".githooks", 0o755); err != nil {
		return err
	}

	hookPath := filepath.Join(".githooks", "post-checkout")
	hookContent := `#!/usr/bin/env sh
set -eu

if [ "${EINAR_HOOK_BYPASS:-}" = "1" ]; then
  exit 0
fi

if [ ! -f ".einar/config.json" ]; then
  exit 0
fi

locked_branch=$(grep -o '"workspaceBranch"[[:space:]]*:[[:space:]]*"[^"]*"' .einar/config.json | sed 's/.*"workspaceBranch"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true)
if [ -z "${locked_branch:-}" ]; then
  exit 0
fi

current_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
if [ -z "${current_branch:-}" ] || [ "$current_branch" = "HEAD" ]; then
  exit 0
fi

if [ "$current_branch" != "$locked_branch" ]; then
  echo "❌ Checkout bloqueado por Einar workspace lock"
  echo "   workspaceBranch: $locked_branch"
  echo "   branch actual:    $current_branch"
  echo "   Revirtiendo checkout para evitar context switch..."
  EINAR_HOOK_BYPASS=1 git checkout -q "$locked_branch" || {
    echo "⚠️  No se pudo revertir automáticamente. Vuelve manualmente: git checkout $locked_branch"
    exit 1
  }
  echo "✅ Workspace restaurado en '$locked_branch'"
  exit 1
fi

exit 0
`

	if err := os.WriteFile(hookPath, []byte(hookContent), 0o755); err != nil {
		return err
	}

	cmd := exec.Command("git", "config", "core.hooksPath", ".githooks")
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("git config core.hooksPath falló: %s", msg)
		}
		return err
	}

	fmt.Println("✅ Hook git instalado para bloquear checkout/switch en workspace")
	return nil
}

func isGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func ensureProjectGitRepository() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	wd = filepath.Clean(wd)

	if isGitRepository() {
		cmd := exec.Command("git", "rev-parse", "--show-toplevel")
		out, err := cmd.CombinedOutput()
		if err == nil {
			top := filepath.Clean(strings.TrimSpace(string(out)))
			if top == wd {
				return nil
			}
		}
		// Está dentro de otro repo: creamos repo propio anidado para aislar workspace.
	}

	init := exec.Command("git", "init", "-b", "main")
	if out, err := init.CombinedOutput(); err != nil {
		// Fallback para versiones viejas de git sin -b
		fallback := exec.Command("git", "init")
		if fbOut, fbErr := fallback.CombinedOutput(); fbErr != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = strings.TrimSpace(string(fbOut))
			}
			if msg != "" {
				return fmt.Errorf("git init falló: %s", msg)
			}
			return fbErr
		}
		checkout := exec.Command("git", "checkout", "-b", "main")
		_ = checkout.Run()
		fmt.Println("✅ Repositorio Git inicializado en main (fallback)")
		return nil
	}

	fmt.Println("✅ Repositorio Git inicializado en main")
	return nil
}

func currentGitBranch() (string, bool) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return "", false
	}
	return branch, true
}

func ensureGoModTidy() error {
	cmd := exec.Command("go", "mod", "tidy")
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("go mod tidy falló: %s", msg)
		}
		return err
	}
	fmt.Println("✅ go mod tidy ejecutado")
	return nil
}

func ensureInitialGitCommit(skip bool) error {
	if skip || !isGitRepository() {
		return nil
	}

	if err := exec.Command("git", "rev-parse", "--verify", "HEAD").Run(); err == nil {
		return nil
	}

	add := exec.Command("git", "add", ".")
	if out, err := add.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("git add falló: %s", msg)
		}
		return err
	}

	commit := exec.Command("git", "commit", "-m", "chore: initial scaffold by einarc")
	if out, err := commit.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(msg), "author identity unknown") || strings.Contains(strings.ToLower(msg), "please tell me who you are") {
			return fmt.Errorf("git commit requiere user.name/user.email configurados")
		}
		if msg != "" {
			return fmt.Errorf("git commit inicial falló: %s", msg)
		}
		return err
	}

	fmt.Println("✅ Commit inicial Git creado")
	return nil
}

func ensureWorkspaceBranchLock(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if !isGitRepositoryRoot() {
		return nil
	}
	currentBranch, ok := currentGitBranch()
	if !ok {
		return nil
	}
	locked := strings.TrimSpace(cfg.WorkspaceBranch)
	if locked == "" {
		cfg.WorkspaceBranch = currentBranch
		if err := saveProjectConfig(*cfg); err != nil {
			return fmt.Errorf("no se pudo persistir workspaceBranch: %w", err)
		}
		return nil
	}
	if locked != currentBranch {
		return fmt.Errorf("branch inválida para este workspace: esperada=%q actual=%q. Evita git switch/checkout en este workspace", locked, currentBranch)
	}
	return nil
}

func isGitRepositoryRoot() bool {
	wd, err := os.Getwd()
	if err != nil {
		return false
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	top := filepath.Clean(strings.TrimSpace(string(out)))
	return top != "" && filepath.Clean(wd) == top
}

type workspaceStateEntry struct {
	ProjectSlug        string `json:"projectSlug"`
	WorkspaceBranch    string `json:"workspaceBranch"`
	MutagenDestination string `json:"mutagenDestination,omitempty"`
	MutagenSessionName string `json:"mutagenSessionName,omitempty"`
	OwnerID            string `json:"ownerId"`
	UpdatedAt          string `json:"updatedAt"`
}

type workspaceStateFile struct {
	Entries []workspaceStateEntry `json:"entries"`
}

func workspaceStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".einar", "workspaces-state.json"), nil
}

func currentWorkspaceOwnerID() string {
	host, _ := os.Hostname()
	user := os.Getenv("USERNAME")
	if strings.TrimSpace(user) == "" {
		user = os.Getenv("USER")
	}
	if strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	if strings.TrimSpace(user) == "" {
		user = "unknown-user"
	}
	return user + "@" + host
}

func loadWorkspaceState() (workspaceStateFile, string, error) {
	path, err := workspaceStatePath()
	if err != nil {
		return workspaceStateFile{}, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceStateFile{}, path, nil
		}
		return workspaceStateFile{}, path, err
	}
	var st workspaceStateFile
	if len(strings.TrimSpace(string(b))) == 0 {
		return workspaceStateFile{}, path, nil
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return workspaceStateFile{}, path, err
	}
	return st, path, nil
}

func saveWorkspaceState(path string, st workspaceStateFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func ensureWorkspaceOwnership(cfg *config.Config, forceTakeover bool) error {
	if cfg == nil {
		return nil
	}
	project := strings.TrimSpace(cfg.LastProjectSlug)
	branch := strings.TrimSpace(cfg.WorkspaceBranch)
	if project == "" || branch == "" {
		return nil
	}
	owner := currentWorkspaceOwnerID()
	state, statePath, err := loadWorkspaceState()
	if err != nil {
		return fmt.Errorf("no se pudo leer estado global de workspaces: %w", err)
	}

	destination := strings.TrimSpace(cfg.MutagenDestination)
	session := strings.TrimSpace(cfg.MutagenSessionName)
	idx := -1
	for i := range state.Entries {
		e := state.Entries[i]
		if e.ProjectSlug == project && e.WorkspaceBranch == branch {
			idx = i
			continue
		}
		if destination != "" && strings.EqualFold(strings.TrimSpace(e.MutagenDestination), destination) {
			if !forceTakeover {
				return fmt.Errorf("workspace en uso: mutagen destination ya reservado por %s (%s/%s). Usa --force-workspace-takeover", e.OwnerID, e.ProjectSlug, e.WorkspaceBranch)
			}
		}
		if session != "" && strings.EqualFold(strings.TrimSpace(e.MutagenSessionName), session) {
			if !forceTakeover {
				return fmt.Errorf("workspace en uso: mutagen session ya reservada por %s (%s/%s). Usa --force-workspace-takeover", e.OwnerID, e.ProjectSlug, e.WorkspaceBranch)
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entry := workspaceStateEntry{
		ProjectSlug:        project,
		WorkspaceBranch:    branch,
		MutagenDestination: destination,
		MutagenSessionName: session,
		OwnerID:            owner,
		UpdatedAt:          now,
	}
	if idx >= 0 {
		existing := state.Entries[idx]
		if strings.TrimSpace(existing.OwnerID) != "" && existing.OwnerID != owner && !forceTakeover {
			return fmt.Errorf("workspace %s/%s está siendo usado por %s. Usa --force-workspace-takeover para tomar control", project, branch, existing.OwnerID)
		}
		state.Entries[idx] = entry
	} else {
		state.Entries = append(state.Entries, entry)
	}

	if err := saveWorkspaceState(statePath, state); err != nil {
		return fmt.Errorf("no se pudo guardar estado global de workspaces: %w", err)
	}
	return nil
}

func gitBranchExists(branch string) bool {
	b := strings.TrimSpace(branch)
	if b == "" {
		return false
	}
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+b)
	if err := cmd.Run(); err == nil {
		return true
	}
	cmd = exec.Command("git", "ls-remote", "--heads", "origin", b)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func setupAndStartRemoteAir(cfg *config.Config) error {
	if err := ensureWorkspaceBranchLock(cfg); err != nil {
		return err
	}
	if err := ensureWorkspaceOwnership(cfg, false); err != nil {
		return err
	}
	if err := ensureLocalAirConfig(); err != nil {
		return err
	}
	destination := strings.TrimSpace(initMutagenDestination)
	if destination == "" {
		destination = strings.TrimSpace(cfg.MutagenDestination)
	}
	destination = normalizeMutagenDestinationForProject(destination)
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return fmt.Errorf("no se pudo resolver destino remoto para air")
	}

	installCmd := `if ! command -v go >/dev/null 2>&1; then echo "missing go"; exit 21; fi; if ! command -v air >/dev/null 2>&1 && [ ! -x "$HOME/go/bin/air" ]; then go install github.com/air-verse/air@latest; fi; if ! command -v templ >/dev/null 2>&1 && [ ! -x "$HOME/go/bin/templ" ]; then go install github.com/a-h/templ/cmd/templ@v0.3.1020; fi; if command -v air >/dev/null 2>&1; then echo "air ready: $(command -v air)"; elif [ -x "$HOME/go/bin/air" ]; then echo "air ready: $HOME/go/bin/air"; else echo "missing air after install"; exit 22; fi; if command -v templ >/dev/null 2>&1; then echo "templ ready: $(command -v templ)"; elif [ -x "$HOME/go/bin/templ" ]; then echo "templ ready: $HOME/go/bin/templ"; else echo "missing templ after install"; exit 23; fi`
	msg, err := runSSHScriptWithTimeout(target, installCmd, 4*time.Minute)
	if err != nil {
		if msg != "" {
			if strings.Contains(msg, "missing go") {
				return fmt.Errorf("la VM no tiene Go instalado; instala Go y reintenta")
			}
			return fmt.Errorf("falló instalación/verificación de air: %s", msg)
		}
		return fmt.Errorf("falló instalación/verificación de air: %w", err)
	}

	remoteEnsureAirToml := fmt.Sprintf("cd %q && cat > .air.toml <<'EOF'\n%s\nEOF\n echo 'synced .air.toml remotely'", remotePath, airTomlContent())
	msg, err = runSSHScriptWithTimeout(target, remoteEnsureAirToml, 45*time.Second)
	if err != nil {
		if msg != "" {
			return fmt.Errorf("falló preparación remota de .air.toml: %s", msg)
		}
		return fmt.Errorf("falló preparación remota de .air.toml: %w", err)
	}

	startCmd := fmt.Sprintf(`cd %q && mkdir -p tmp && AIR_BIN="$(command -v air || echo $HOME/go/bin/air)" && if [ ! -f .air.toml ]; then echo "missing .air.toml"; exit 12; fi && if [ -f .air.pid ] && kill -0 "$(cat .air.pid)" 2>/dev/null; then kill "$(cat .air.pid)" 2>/dev/null || true; rm -f .air.pid; fi && nohup env PORT=8000 "$AIR_BIN" -c .air.toml > .air.log 2>&1 & echo $! > .air.pid && sleep 1 && if [ -f .air.pid ] && kill -0 "$(cat .air.pid)" 2>/dev/null; then echo "air started pid=$(cat .air.pid)"; else echo "air exited"; tail -n 120 .air.log 2>/dev/null || true; exit 13; fi`, remotePath)
	msg, err = runSSHScriptWithTimeout(target, startCmd, 60*time.Second)
	if err != nil {
		low := strings.ToLower(msg)
		if strings.Contains(low, "air started pid=") || strings.Contains(low, "air ya corriendo") {
			fmt.Printf("ℹ️  Air reportó estado saludable: %s\n", msg)
			fmt.Printf("✅ Air iniciado automáticamente en %s:%s\n", target, remotePath)
			return nil
		}
		if msg != "" {
			return fmt.Errorf("falló inicio de air remoto: %s", msg)
		}
		return fmt.Errorf("falló inicio de air remoto: %w", err)
	}
	fmt.Printf("✅ Air iniciado automáticamente en %s:%s\n", target, remotePath)
	return nil
}

func setupAndStartRemoteAgentSidecars(cfg *config.Config) error {
	if err := ensureWorkspaceBranchLock(cfg); err != nil {
		return err
	}
	if err := ensureWorkspaceOwnership(cfg, false); err != nil {
		return err
	}
	destination := strings.TrimSpace(initMutagenDestination)
	if destination == "" {
		destination = strings.TrimSpace(cfg.MutagenDestination)
	}
	destination = normalizeMutagenDestinationForProject(destination)
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return fmt.Errorf("no se pudo resolver destino remoto para sidecars del agente")
	}

	checkCmd := `set -eu
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/bus}"
[ -S "$XDG_RUNTIME_DIR/bus" ] || { echo "missing user bus at $XDG_RUNTIME_DIR/bus"; exit 41; }
systemctl --user status >/dev/null 2>&1 && echo "systemd-user ready"`
	msg, err := runSSHScriptWithTimeout(target, checkCmd, 20*time.Second)
	if err != nil {
		if strings.TrimSpace(msg) != "" {
			return fmt.Errorf("systemd --user no disponible: %s", strings.TrimSpace(msg))
		}
		return fmt.Errorf("systemd --user no disponible: %w", err)
	}
	_ = msg
	remoteRoot := strings.TrimSpace(remotePath)

	installCmd := fmt.Sprintf(`set -eu
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/bus}"
[ -S "$XDG_RUNTIME_DIR/bus" ] || { echo "missing user bus at $XDG_RUNTIME_DIR/bus"; exit 41; }
PROJECT_DIR=%s
SYSTEMD_DIR="$HOME/.config/systemd/user"
mkdir -p "$SYSTEMD_DIR" "$PROJECT_DIR/tmp" "$PROJECT_DIR/bin"
chmod +x "$PROJECT_DIR/scripts/systemd/run-agent-worker.sh" "$PROJECT_DIR/scripts/systemd/run-bff.sh"
cat > "$SYSTEMD_DIR/agent-worker.service" <<'EOF'
[Unit]
Description=Agent worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s/scripts/systemd/run-agent-worker.sh
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
EOF
cat > "$SYSTEMD_DIR/bff.service" <<'EOF'
[Unit]
Description=BFF gateway
After=network-online.target agent-worker.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s/scripts/systemd/run-bff.sh
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
EOF
systemctl --user daemon-reload
systemctl --user enable --now agent-worker.service
systemctl --user enable --now bff.service
for unit in agent-worker.service bff.service; do
  systemctl --user is-active --quiet "$unit" || {
    systemctl --user status "$unit" --no-pager || true
    exit 31
  }
done
if command -v curl >/dev/null 2>&1; then
  for i in $(seq 1 20); do
    if curl -fsS http://127.0.0.1:18080/agent/healthz >/dev/null 2>&1 && curl -fsS http://127.0.0.1:8000/agent/healthz >/dev/null 2>&1; then
      echo "agent sidecars ready"
      exit 0
    fi
    sleep 1
  done
  echo "sidecars started but healthz did not answer in time"
  systemctl --user status agent-worker.service bff.service --no-pager || true
  exit 32
fi
`, shellQuote(remoteRoot), remoteRoot, remoteRoot, remoteRoot, remoteRoot)
	msg, err = runSSHScriptWithTimeout(target, installCmd, 90*time.Second)
	if err != nil {
		if strings.TrimSpace(msg) != "" {
			return fmt.Errorf("falló instalación/inicio de sidecars: %s", strings.TrimSpace(msg))
		}
		return fmt.Errorf("falló instalación/inicio de sidecars: %w", err)
	}
	fmt.Printf("✅ Sidecars systemd instalados en %s:%s\n", target, remotePath)
	return nil
}

func ensureRemoteWedeReady(cfg *config.Config) error {
	if err := ensureWorkspaceBranchLock(cfg); err != nil {
		return err
	}
	if err := ensureWorkspaceOwnership(cfg, false); err != nil {
		return err
	}
	destination := strings.TrimSpace(initMutagenDestination)
	if destination == "" {
		destination = strings.TrimSpace(cfg.MutagenDestination)
	}
	destination = normalizeMutagenDestinationForProject(destination)
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return fmt.Errorf("no se pudo resolver destino remoto para wede")
	}
	if strings.TrimSpace(remotePath) == "" {
		remotePath = "$HOME"
	}

	downloadURL, err := resolveGitHubReleaseAssetDownloadURL("Ignaciojeria", "wede", "v1.0.2", "linux", "amd64")
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`set -eu
PROJECT_DIR=%s
CONFIG_DIR="$HOME/.config/wede"
CONFIG_FILE="$CONFIG_DIR/wede.config.json"
PROJECT_CONFIG_FILE="$PROJECT_DIR/wede.config.json"
WEDE_PASSWORD="wede-dev"
mkdir -p "$HOME/.local/bin" "$HOME/.cache/einar" "$HOME/.einar" "$CONFIG_DIR" "$PROJECT_DIR"
if ! grep -F 'export PATH="$HOME/.local/bin:$PATH"' "$HOME/.bashrc" >/dev/null 2>&1; then
  printf '\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "$HOME/.bashrc"
fi
tmpdir=$(mktemp -d)
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT
asset_url=%s
asset_file="$tmpdir/$(basename "$asset_url")"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$asset_url" -o "$asset_file"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$asset_file" "$asset_url"
else
  echo "missing curl/wget"
  exit 21
fi
case "$asset_file" in
  *.tar.gz|*.tgz)
    tar -xzf "$asset_file" -C "$tmpdir"
    ;;
  *.zip)
    if ! command -v unzip >/dev/null 2>&1; then
      echo "missing unzip"
      exit 22
    fi
    unzip -q "$asset_file" -d "$tmpdir"
    ;;
  *)
    chmod +x "$asset_file"
    install -m 0755 "$asset_file" "$HOME/.local/bin/wede"
    ;;
esac
if [ ! -x "$HOME/.local/bin/wede" ]; then
  found=$(find "$tmpdir" -type f \( -name wede -o -name 'wede-*' \) | head -n 1 || true)
  if [ -z "$found" ]; then
    echo "wede binary not found in release asset"
    exit 23
  fi
  install -m 0755 "$found" "$HOME/.local/bin/wede"
fi
cat > "$CONFIG_FILE" <<EOF
{
  "password": "$WEDE_PASSWORD",
  "port": "9090"
}
EOF
cat > "$PROJECT_CONFIG_FILE" <<EOF
{
  "password": "$WEDE_PASSWORD",
  "port": "9090"
}
EOF
if [ -f "$HOME/.einar/wede.pid" ] && kill -0 "$(cat "$HOME/.einar/wede.pid")" 2>/dev/null; then
  kill "$(cat "$HOME/.einar/wede.pid")" 2>/dev/null || true
  sleep 1
fi
cd "$PROJECT_DIR"
nohup "$HOME/.local/bin/wede" "$PROJECT_DIR" -p 9090 > "$HOME/.einar/wede.log" 2>&1 &
echo $! > "$HOME/.einar/wede.pid"
sleep 2
if ! kill -0 "$(cat "$HOME/.einar/wede.pid")" 2>/dev/null; then
  tail -n 120 "$HOME/.einar/wede.log" 2>/dev/null || true
  exit 24
fi
if command -v curl >/dev/null 2>&1; then
  curl -fsSI http://127.0.0.1:9090/ >/dev/null || {
    tail -n 120 "$HOME/.einar/wede.log" 2>/dev/null || true
    exit 25
  }
fi
echo "wede ready: $HOME/.local/bin/wede project=$PROJECT_DIR config=$CONFIG_FILE project_config=$PROJECT_CONFIG_FILE"
`, shellQuote(remotePath), shellQuote(downloadURL))
	msg, err := runSSHScriptWithTimeout(target, script, 4*time.Minute)
	if err != nil {
		if strings.TrimSpace(msg) != "" {
			return fmt.Errorf("falló instalación/inicio de wede: %s", strings.TrimSpace(msg))
		}
		return fmt.Errorf("falló instalación/inicio de wede: %w", err)
	}
	fmt.Printf("✅ Wede listo en %s\n", target)
	return nil
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	Assets []githubReleaseAsset `json:"assets"`
}

func resolveGitHubReleaseAssetDownloadURL(owner, repo, tag, goos, goarch string) (string, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("github release request falló: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	wantOS := strings.ToLower(strings.TrimSpace(goos))
	wantArch := strings.ToLower(strings.TrimSpace(goarch))
	for _, asset := range release.Assets {
		name := strings.ToLower(strings.TrimSpace(asset.Name))
		if name == "" || strings.TrimSpace(asset.BrowserDownloadURL) == "" {
			continue
		}
		if !strings.Contains(name, wantOS) {
			continue
		}
		if !(strings.Contains(name, wantArch) || (wantArch == "amd64" && strings.Contains(name, "x86_64"))) {
			continue
		}
		if strings.Contains(name, ".sha") || strings.Contains(name, "checksum") || strings.Contains(name, "checksums") {
			continue
		}
		return strings.TrimSpace(asset.BrowserDownloadURL), nil
	}
	return "", fmt.Errorf("no se encontró asset de wede %s/%s para release %s", goos, goarch, tag)
}

func ensureMutagenOnWindows() error {
	if _, err := exec.LookPath("mutagen"); err == nil {
		return nil
	}
	if p, _ := localMutagenPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return nil
		}
	}

	if runtime.GOOS == "windows" {
		// Intento 1: winget
		if _, err := exec.LookPath("winget"); err == nil {
			cmd := exec.Command("winget", "install", "--id", "MutagenIO.Mutagen", "-e", "--accept-package-agreements", "--accept-source-agreements")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			if _, err := exec.LookPath("mutagen"); err == nil {
				return nil
			}
		}

		// Intento 2: choco
		if _, err := exec.LookPath("choco"); err == nil {
			cmd := exec.Command("choco", "install", "mutagen", "-y")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			if _, err := exec.LookPath("mutagen"); err == nil {
				return nil
			}
		}
	}

	// Fallback para cualquier SO: descarga directa del asset correcto (GOOS/GOARCH).
	if err := downloadMutagenLocal(); err == nil {
		return nil
	} else {
		fmt.Printf("⚠️  Fallback descarga Mutagen falló: %v\n", err)
	}

	return fmt.Errorf("mutagen no está en PATH y no se pudo instalar automáticamente")
}

func writeSSHPrivateKey(projectSlug, destination, key string) error {
	path, err := config.ConfigPath()
	if err != nil {
		return err
	}
	keyDir := filepath.Dir(path)
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		return fmt.Errorf("no se pudo crear directorio .einar: %w", err)
	}
	keyPath := filepath.Join(keyDir, "id_ed25519")
	content := strings.TrimSpace(key) + "\n"
	if err := os.WriteFile(keyPath, []byte(content), 0o600); err != nil {
		return err
	}
	if err := enforceSSHPrivateKeyPermissions(keyPath); err != nil {
		fmt.Printf("⚠️  No se pudieron endurecer permisos de clave SSH (%s): %v\n", keyPath, err)
	}
	if err := configureSSHHostIdentity(projectSlug, destination, keyPath); err != nil {
		fmt.Printf("⚠️  No se pudo configurar ~/.ssh/config para clave dedicada: %v\n", err)
	}
	fmt.Printf("✅ SSH private key guardada en %s\n", keyPath)
	return nil
}

func writeProjectSecretsBundle(slug, sshPrivateKey, projectAPIToken, dbPassword, machineClientID, machineClientSecret, casdoorAdminClientSecret, aigwAPIKey string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	baseDir := filepath.Join(home, ".einar", "projects", strings.TrimSpace(slug), "secrets")
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return err
	}
	writeIf := func(name, value string) error {
		v := strings.TrimSpace(value)
		if v == "" {
			return nil
		}
		return os.WriteFile(filepath.Join(baseDir, name), []byte(v+"\n"), 0o600)
	}
	if err := writeIf("ssh-private-key", sshPrivateKey); err != nil {
		return err
	}
	if err := writeIf("api-token", projectAPIToken); err != nil {
		return err
	}
	if err := writeIf("db-password", dbPassword); err != nil {
		return err
	}
	if err := writeIf("machine-client-id", machineClientID); err != nil {
		return err
	}
	if err := writeIf("machine-client-secret", machineClientSecret); err != nil {
		return err
	}
	if err := writeIf("casdoor-admin-client-secret", casdoorAdminClientSecret); err != nil {
		return err
	}
	if err := writeIf("aigw-api-key", aigwAPIKey); err != nil {
		return err
	}
	return nil
}

func configureSSHHostIdentity(projectSlug, destination, identityFile string) error {
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	host := target
	if i := strings.Index(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return err
	}
	cfgPath := filepath.Join(sshDir, "config")
	existing := ""
	if b, err := os.ReadFile(cfgPath); err == nil {
		existing = string(b)
	}
	marker := fmt.Sprintf("# einarc-project:%s", strings.TrimSpace(projectSlug))
	block := fmt.Sprintf("%s\nHost %s\n  IdentityFile %s\n  IdentitiesOnly yes\n", marker, host, identityFile)
	if strings.Contains(existing, marker) {
		return nil
	}
	f, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if strings.TrimSpace(existing) != "" && !strings.HasSuffix(existing, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(block)
	return err
}

func normalizeProjectAuth(resp *api.CreateProjectResponse) *api.ProjectAuth {
	if resp == nil {
		return nil
	}

	auth := resp.Identity
	if auth == nil {
		auth = resp.Auth
	}
	if auth == nil {
		return nil
	}

	issuer := strings.TrimRight(strings.TrimSpace(auth.Issuer), "/")
	if issuer == "" || strings.TrimSpace(auth.ClientID) == "" {
		return nil
	}

	scopes := auth.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	clientSecret := strings.TrimSpace(auth.ClientSecret)
	if clientSecret == "" && resp.Secrets != nil {
		clientSecret = strings.TrimSpace(resp.Secrets.OIDCClientSecret)
	}
	clientSecretRef := strings.TrimSpace(auth.ClientSecretRef)
	if clientSecretRef == "" && resp.Secrets != nil {
		clientSecretRef = strings.TrimSpace(resp.Secrets.OIDCClientSecretRef)
	}

	normalized := &api.ProjectAuth{
		Type:                   firstNonEmptyTrimmed(strings.TrimSpace(auth.Type), "oidc"),
		Provider:               firstNonEmptyTrimmed(strings.TrimSpace(auth.Provider), "casdoor"),
		Issuer:                 issuer,
		DiscoveryURL:           firstNonEmptyTrimmed(strings.TrimSpace(auth.DiscoveryURL), issuer+"/.well-known/openid-configuration"),
		JWKSURI:                firstNonEmptyTrimmed(strings.TrimSpace(auth.JWKSURI), issuer+"/api/certs"),
		AuthorizationEndpoint:  firstNonEmptyTrimmed(strings.TrimSpace(auth.AuthorizationEndpoint), issuer+"/login/oauth/authorize"),
		TokenEndpoint:          firstNonEmptyTrimmed(strings.TrimSpace(auth.TokenEndpoint), issuer+"/api/login/oauth/access_token"),
		UserinfoEndpoint:       firstNonEmptyTrimmed(strings.TrimSpace(auth.UserinfoEndpoint), issuer+"/api/userinfo"),
		ClientID:               strings.TrimSpace(auth.ClientID),
		ClientSecret:           clientSecret,
		ClientSecretRef:        clientSecretRef,
		RedirectURI:            strings.TrimSpace(auth.RedirectURI),
		LogoutURI:              strings.TrimSpace(auth.LogoutURI),
		PostLogoutRedirectURI:  strings.TrimSpace(auth.PostLogoutRedirectURI),
		Scopes:                 scopes,
		LoginURL:               strings.TrimSpace(auth.LoginURL),
		GoogleLoginURL:         strings.TrimSpace(auth.GoogleLoginURL),
		UpstreamGoogleClientID: strings.TrimSpace(auth.UpstreamGoogleClientID),
		Organization:           strings.TrimSpace(auth.Organization),
		Application:            strings.TrimSpace(auth.Application),
	}
	if normalized.LoginURL == "" {
		normalized.LoginURL = buildOIDCLoginURL(normalized.AuthorizationEndpoint, normalized.ClientID, normalized.RedirectURI, normalized.Scopes)
	}
	return normalized
}

func normalizeProjectCasdoorAdmin(resp *api.CreateProjectResponse) *api.ProjectCasdoorAdmin {
	if resp == nil || resp.IdentityExtensions == nil || resp.IdentityExtensions.CasdoorAdmin == nil {
		return nil
	}
	admin := resp.IdentityExtensions.CasdoorAdmin
	clientSecret := strings.TrimSpace(admin.ClientSecret)
	if clientSecret == "" && resp.Secrets != nil {
		clientSecret = strings.TrimSpace(resp.Secrets.CasdoorAdminClientSecret)
	}
	clientSecretRef := strings.TrimSpace(admin.ClientSecretRef)
	if clientSecretRef == "" && resp.Secrets != nil {
		clientSecretRef = strings.TrimSpace(resp.Secrets.CasdoorAdminClientSecretRef)
	}
	if strings.TrimSpace(admin.TokenEndpoint) == "" || strings.TrimSpace(admin.ClientID) == "" {
		return nil
	}
	return &api.ProjectCasdoorAdmin{
		Provider:         firstNonEmptyTrimmed(strings.TrimSpace(admin.Provider), "casdoor"),
		APIBaseURL:       strings.TrimSpace(admin.APIBaseURL),
		GatewayURL:       strings.TrimSpace(admin.GatewayURL),
		Organization:     strings.TrimSpace(admin.Organization),
		Application:      strings.TrimSpace(admin.Application),
		ClientID:         strings.TrimSpace(admin.ClientID),
		ClientSecret:     clientSecret,
		ClientSecretRef:  clientSecretRef,
		TokenEndpoint:    strings.TrimSpace(admin.TokenEndpoint),
		Scopes:           admin.Scopes,
		TenantScopedOnly: admin.TenantScopedOnly,
	}
}

func normalizeProjectMachineAuth(resp *api.CreateProjectResponse) *api.ProjectMachineAuth {
	if resp == nil || resp.MachineAuth == nil {
		return nil
	}

	auth := resp.MachineAuth
	clientSecret := strings.TrimSpace(auth.ClientSecret)
	if clientSecret == "" && resp.Secrets != nil {
		clientSecret = strings.TrimSpace(resp.Secrets.MachineClientSecret)
	}
	clientSecretRef := strings.TrimSpace(auth.ClientSecretRef)
	if clientSecretRef == "" && resp.Secrets != nil {
		clientSecretRef = strings.TrimSpace(resp.Secrets.MachineClientSecretRef)
	}
	tokenEndpoint := strings.TrimSpace(auth.TokenEndpoint)
	clientID := strings.TrimSpace(auth.ClientID)
	if tokenEndpoint == "" || clientID == "" {
		return nil
	}

	scopes := auth.Scopes
	if len(scopes) == 0 {
		scopes = nil
	}

	return &api.ProjectMachineAuth{
		GrantType:       firstNonEmptyTrimmed(strings.TrimSpace(auth.GrantType), "client_credentials"),
		TokenEndpoint:   tokenEndpoint,
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		ClientSecretRef: clientSecretRef,
		Audience:        strings.TrimSpace(auth.Audience),
		Scopes:          scopes,
	}
}

func normalizeProjectAIGateway(resp *api.CreateProjectResponse) *api.ProjectAIGateway {
	if resp == nil || resp.AIGateway == nil {
		return nil
	}
	gw := resp.AIGateway
	clientID := strings.TrimSpace(gw.ClientID)
	apiBaseURL := strings.TrimSpace(gw.APIBaseURL)
	if clientID == "" && apiBaseURL == "" {
		return nil
	}
	apiKey := strings.TrimSpace(gw.APIKey)
	if apiKey == "" && resp.Secrets != nil {
		apiKey = strings.TrimSpace(resp.Secrets.AIGWAPIKey)
	}
	apiKeyRef := strings.TrimSpace(gw.APIKeyRef)
	if apiKeyRef == "" && resp.Secrets != nil {
		apiKeyRef = strings.TrimSpace(resp.Secrets.AIGWAPIKeyRef)
	}
	return &api.ProjectAIGateway{
		Provider:    firstNonEmptyTrimmed(strings.TrimSpace(gw.Provider), "aigateway"),
		APIBaseURL:  apiBaseURL,
		ClientID:    clientID,
		ClientName:  strings.TrimSpace(gw.ClientName),
		ClientEmail: strings.TrimSpace(gw.ClientEmail),
		KeyLabel:    strings.TrimSpace(gw.KeyLabel),
		KeyID:       strings.TrimSpace(gw.KeyID),
		KeyPrefix:   strings.TrimSpace(gw.KeyPrefix),
		APIKey:      apiKey,
		APIKeyRef:   apiKeyRef,
	}
}

func buildOIDCLoginURL(authorizationEndpoint, clientID, redirectURI string, scopes []string) string {
	if strings.TrimSpace(authorizationEndpoint) == "" || strings.TrimSpace(clientID) == "" || strings.TrimSpace(redirectURI) == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(authorizationEndpoint))
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("client_id", strings.TrimSpace(clientID))
	q.Set("redirect_uri", strings.TrimSpace(redirectURI))
	q.Set("response_type", "code")
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func saveProjectConfig(cfg config.Config) error {
	cfg.APIURL = ""
	cfg.Token = ""
	cfg.RefreshToken = ""
	return config.Save(cfg)
}

func materializeProjectEnv(cfg config.Config) error {
	databaseURLForRuntime := strings.TrimSpace(cfg.ProjectDatabaseURL)
	if databaseURLForRuntime != "" {
		if rewritten, err := localDatabaseURL(databaseURLForRuntime, defaultRemoteDBListenHost, defaultRemoteDBListenPort); err == nil {
			databaseURLForRuntime = rewritten
		}
	}
	entries := map[string]string{
		"PROJECT_NAME":                     strings.TrimSpace(firstNonEmpty(cfg.LastProjectSlug, cfg.LastProjectID)),
		"DATABASE_URL":                     databaseURLForRuntime,
		"OIDC_TYPE":                        strings.TrimSpace(cfg.OIDCType),
		"OIDC_PROVIDER":                    strings.TrimSpace(cfg.OIDCProvider),
		"OIDC_ISSUER":                      strings.TrimSpace(cfg.OIDCIssuer),
		"OIDC_DISCOVERY_URL":               strings.TrimSpace(cfg.OIDCDiscoveryURL),
		"OIDC_JWKS_URI":                    strings.TrimSpace(cfg.OIDCJWKSURI),
		"OIDC_AUTHORIZATION_ENDPOINT":      strings.TrimSpace(cfg.OIDCAuthorizationEndpoint),
		"OIDC_TOKEN_ENDPOINT":              strings.TrimSpace(cfg.OIDCTokenEndpoint),
		"OIDC_USERINFO_ENDPOINT":           strings.TrimSpace(cfg.OIDCUserinfoEndpoint),
		"OIDC_CLIENT_ID":                   strings.TrimSpace(cfg.OIDCClientID),
		"OIDC_CLIENT_SECRET":               strings.TrimSpace(cfg.OIDCClientSecret),
		"OIDC_CLIENT_SECRET_REF":           strings.TrimSpace(cfg.OIDCClientSecretRef),
		"OIDC_REDIRECT_URI":                strings.TrimSpace(cfg.OIDCRedirectURI),
		"OIDC_LOGOUT_URI":                  strings.TrimSpace(cfg.OIDCLogoutURI),
		"OIDC_POST_LOGOUT_REDIRECT_URI":    strings.TrimSpace(cfg.OIDCPostLogoutRedirectURI),
		"OIDC_SCOPES":                      strings.TrimSpace(cfg.OIDCScopes),
		"OIDC_LOGIN_URL":                   strings.TrimSpace(cfg.OIDCLoginURL),
		"OIDC_GOOGLE_LOGIN_URL":            strings.TrimSpace(cfg.OIDCGoogleLoginURL),
		"OIDC_UPSTREAM_GOOGLE_CLIENT_ID":   strings.TrimSpace(cfg.OIDCUpstreamGoogleClientID),
		"CASDOOR_ADMIN_API_BASE_URL":       strings.TrimSpace(cfg.CasdoorAdminAPIBaseURL),
		"CASDOOR_ADMIN_GATEWAY_URL":        strings.TrimSpace(cfg.CasdoorAdminGatewayURL),
		"CASDOOR_ADMIN_ORGANIZATION":       strings.TrimSpace(cfg.CasdoorOrg),
		"CASDOOR_ADMIN_APPLICATION":        strings.TrimSpace(cfg.CasdoorApplication),
		"CASDOOR_ADMIN_CLIENT_ID":          strings.TrimSpace(cfg.CasdoorAdminClientID),
		"CASDOOR_ADMIN_CLIENT_SECRET":      strings.TrimSpace(cfg.CasdoorAdminClientSecret),
		"CASDOOR_ADMIN_CLIENT_SECRET_REF":  strings.TrimSpace(cfg.CasdoorAdminClientSecretRef),
		"CASDOOR_ADMIN_TOKEN_ENDPOINT":     strings.TrimSpace(cfg.CasdoorAdminTokenEndpoint),
		"CASDOOR_ADMIN_SCOPES":             strings.TrimSpace(cfg.CasdoorAdminScopes),
		"CASDOOR_ADMIN_TENANT_SCOPED_ONLY": fmt.Sprintf("%t", cfg.CasdoorAdminTenantScopedOnly),
		"MACHINE_AUTH_GRANT_TYPE":          strings.TrimSpace(cfg.MachineAuthGrantType),
		"MACHINE_AUTH_TOKEN_ENDPOINT":      strings.TrimSpace(cfg.MachineAuthTokenEndpoint),
		"MACHINE_AUTH_CLIENT_ID":           strings.TrimSpace(cfg.MachineAuthClientID),
		"MACHINE_AUTH_CLIENT_SECRET":       strings.TrimSpace(cfg.MachineAuthClientSecret),
		"MACHINE_AUTH_CLIENT_SECRET_REF":   strings.TrimSpace(cfg.MachineAuthClientSecretRef),
		"MACHINE_AUTH_AUDIENCE":            strings.TrimSpace(cfg.MachineAuthAudience),
		"MACHINE_AUTH_SCOPES":              strings.TrimSpace(cfg.MachineAuthScopes),
		"SYNC_AI_GATEWAY_BASE_URL":         strings.TrimSpace(cfg.AIGatewayAPIBaseURL),
		"SYNC_AI_GATEWAY_API_KEY":          strings.TrimSpace(cfg.AIGatewayAPIKey),
	}

	if entries["OIDC_UPSTREAM_GOOGLE_CLIENT_ID"] == "" && strings.Contains(strings.ToLower(entries["OIDC_GOOGLE_LOGIN_URL"]), "accounts.google.com/signin/oauth") {
		if u, err := url.Parse(strings.TrimSpace(entries["OIDC_GOOGLE_LOGIN_URL"])); err == nil {
			entries["OIDC_UPSTREAM_GOOGLE_CLIENT_ID"] = strings.TrimSpace(u.Query().Get("client_id"))
		}
	}

	hasAny := false
	for _, v := range entries {
		if v != "" {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return nil
	}

	lines := []string{}
	if b, err := os.ReadFile(".env"); err == nil {
		s := bufio.NewScanner(strings.NewReader(string(b)))
		for s.Scan() {
			lines = append(lines, s.Text())
		}
		if err := s.Err(); err != nil {
			return err
		}
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "export ") {
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
			}
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value, ok := entries[key]
			if !ok || value == "" {
				continue
			}
			lines[i] = key + "=" + value
			delete(entries, key)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	for _, key := range []string{"PROJECT_NAME", "DATABASE_URL", "OIDC_TYPE", "OIDC_PROVIDER", "OIDC_ISSUER", "OIDC_DISCOVERY_URL", "OIDC_JWKS_URI", "OIDC_AUTHORIZATION_ENDPOINT", "OIDC_TOKEN_ENDPOINT", "OIDC_USERINFO_ENDPOINT", "OIDC_CLIENT_ID", "OIDC_CLIENT_SECRET", "OIDC_CLIENT_SECRET_REF", "OIDC_REDIRECT_URI", "OIDC_LOGOUT_URI", "OIDC_POST_LOGOUT_REDIRECT_URI", "OIDC_SCOPES", "OIDC_LOGIN_URL", "OIDC_GOOGLE_LOGIN_URL", "OIDC_UPSTREAM_GOOGLE_CLIENT_ID", "CASDOOR_ADMIN_API_BASE_URL", "CASDOOR_ADMIN_GATEWAY_URL", "CASDOOR_ADMIN_ORGANIZATION", "CASDOOR_ADMIN_APPLICATION", "CASDOOR_ADMIN_CLIENT_ID", "CASDOOR_ADMIN_CLIENT_SECRET", "CASDOOR_ADMIN_CLIENT_SECRET_REF", "CASDOOR_ADMIN_TOKEN_ENDPOINT", "CASDOOR_ADMIN_SCOPES", "CASDOOR_ADMIN_TENANT_SCOPED_ONLY", "MACHINE_AUTH_GRANT_TYPE", "MACHINE_AUTH_TOKEN_ENDPOINT", "MACHINE_AUTH_CLIENT_ID", "MACHINE_AUTH_CLIENT_SECRET", "MACHINE_AUTH_CLIENT_SECRET_REF", "MACHINE_AUTH_AUDIENCE", "MACHINE_AUTH_SCOPES", "SYNC_AI_GATEWAY_BASE_URL", "SYNC_AI_GATEWAY_API_KEY"} {
		if value := strings.TrimSpace(entries[key]); value != "" {
			lines = append(lines, key+"="+value)
		}
	}

	content := strings.Join(lines, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(".env", []byte(content), 0o600); err != nil {
		return err
	}
	fmt.Println("✅ .env materializado con configuración OIDC/Casdoor admin/machine auth")
	return nil
}

func setupAndStartMutagen(cfg *config.Config) error {
	if err := ensureWorkspaceBranchLock(cfg); err != nil {
		return err
	}
	if err := ensureWorkspaceOwnership(cfg, false); err != nil {
		return err
	}
	mutagenBin, err := resolveMutagenBinary()
	if err != nil {
		return err
	}

	destination := strings.TrimSpace(initMutagenDestination)
	if destination == "" {
		destination = strings.TrimSpace(cfg.MutagenDestination)
	}
	sessionName := strings.TrimSpace(initMutagenName)
	if sessionName == "" {
		sessionName = strings.TrimSpace(cfg.MutagenSessionName)
	}
	if sessionName == "" {
		sessionName = defaultMutagenSessionName(cfg.LastProjectSlug)
		cfg.MutagenSessionName = sessionName
		_ = saveProjectConfig(*cfg)
	}

	if _, err := os.Stat("mutagen.yml"); os.IsNotExist(err) {
		if destination == "" {
			return fmt.Errorf("falta destino mutagen (usa --mutagen-destination o configura PROJECTS_SYNC_SSH_* en backend)")
		}
		destination = normalizeMutagenDestinationForProject(destination)
		fmt.Printf("ℹ️  Mutagen destination normalizado: %s\n", destination)
		alpha := "."
		if runtime.GOOS != "windows" {
			source, err := os.Getwd()
			if err != nil {
				return err
			}
			absSource, err := filepath.Abs(source)
			if err != nil {
				return err
			}
			alpha = absSource
		}
		if !strings.Contains(destination, "://") && !strings.Contains(destination, ":/") {
			return fmt.Errorf("destino mutagen inválido: %q (debe ser ssh://.../ruta, user@host:/ruta o docker://.../ruta)", destination)
		}
		content := fmt.Sprintf(`sync:
  defaults:
    mode: "two-way-resolved"
    ignore:
      vcs: true
      paths:
        - "node_modules"
        - ".git"

  %s:
    alpha: "%s"
    beta: "%s"
`, sessionName, alpha, destination)
		if err := os.WriteFile("mutagen.yml", []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Println("✅ mutagen.yml generado automáticamente")
	}

	if skipMutagenStart {
		fmt.Println("ℹ️  Se omitió 'mutagen project start' por --skip-mutagen-start")
		return nil
	}
	if err := ensureLocalSSHKeySetup(); err != nil {
		fmt.Printf("⚠️  No se pudo preparar clave SSH local automáticamente: %v\n", err)
	}
	if err := ensureExeDevSSHOnboarding(destination); err != nil {
		return err
	}

	if err := ensureSSHTrustInteractive(destination); err != nil {
		return err
	}
	if err := preflightSSHConnection(destination); err != nil {
		return err
	}
	if err := ensureRemoteMutagenRoot(destination); err != nil {
		return err
	}

	if err := startMutagenProjectWithRetry(mutagenBin); err != nil {
		return err
	}
	fmt.Println("✅ Mutagen sync iniciado (mutagen project start)")
	if err := ensureInitialSyncHealthy(mutagenBin, sessionName, destination); err != nil {
		fmt.Printf("⚠️  Sync aún no está saludable (continuando): %v\n", err)
	}
	if err := triggerTopologyHeartbeatOnce(cfg); err != nil {
		fmt.Printf("⚠️  No se pudo refrescar topología inmediatamente: %v\n", err)
	}
	if err := startTopologyHeartbeatProcess(cfg); err != nil {
		fmt.Printf("⚠️  No se pudo iniciar heartbeat de topología: %v\n", err)
	}
	printMutagenPostInitChecklist(destination, sessionName, strings.TrimSpace(cfg.LastVMHTTPSURL))
	return nil
}

func startMutagenProjectWithRetry(mutagenBin string) error {
	// Evita usar un daemon viejo iniciado con otro binario/carpeta de agentes.
	resetMutagenDaemonState(mutagenBin)

	maxAttempts := 5
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cmd := exec.Command(mutagenBin, "project", "start")
		out, err := cmd.CombinedOutput()
		output := strings.TrimSpace(string(out))
		lowOut := strings.ToLower(output)

		if err == nil {
			if output != "" {
				fmt.Println(output)
			}
			return nil
		}
		if strings.Contains(lowOut, "project already running") {
			fmt.Println("✅ Mutagen ya estaba corriendo para este proyecto")
			return nil
		}

		lastErr = err
		if output != "" {
			fmt.Println(output)
		}

		if strings.Contains(lowOut, "unable to connect to daemon") || strings.Contains(lowOut, "connection timed out") {
			fmt.Printf("ℹ️  Mutagen intento %d/%d: daemon no disponible; reiniciando y reintentando...\n", attempt, maxAttempts)
			resetMutagenDaemonState(mutagenBin)
			startDaemon := exec.Command(mutagenBin, "daemon", "start")
			dOut, dErr := startDaemon.CombinedOutput()
			daemonMsg := strings.TrimSpace(string(dOut))
			if daemonMsg != "" {
				fmt.Println(daemonMsg)
			}
			if dErr != nil {
				lastErr = dErr
			}
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}

		return err
	}

	if lastErr != nil {
		return fmt.Errorf("mutagen project start falló tras varios reintentos: %w", lastErr)
	}
	return fmt.Errorf("mutagen project start falló tras varios reintentos")
}

func resetMutagenDaemonState(mutagenBin string) {
	_ = exec.Command(mutagenBin, "daemon", "stop").Run()
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/IM", "mutagen.exe", "/F").Run()
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		_ = os.Remove(filepath.Join(home, ".mutagen", "daemon", "daemon.lock"))
	}
}

func printMutagenPostInitChecklist(destination, sessionName, vmURL string) {
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		fmt.Println("ℹ️  Verificación manual sugerida: mutagen sync list --long")
		return
	}
	if strings.TrimSpace(sessionName) == "" {
		sessionName = "dev-sync"
	}
	hint := mutagenHintCommand()
	fmt.Println("📋 Checklist de verificación")
	fmt.Printf("   - Sesión: %s\n", sessionName)
	fmt.Printf("   - Destino remoto: %s:%s\n", target, remotePath)
	fmt.Printf("   - Estado: ejecuta '%s sync list --long'\n", hint)
	fmt.Printf("   - VM check: ssh %s \"ls -la %s\"\n", target, remotePath)
	if strings.TrimSpace(vmURL) != "" {
		fmt.Printf("   - HTTP check: curl -I %s\n", strings.TrimSpace(vmURL))
	}
}

func mutagenHintCommand() string {
	p, err := localMutagenPath()
	if err == nil && strings.TrimSpace(p) != "" {
		if runtime.GOOS == "windows" {
			return p
		}
		return p
	}
	return "mutagen"
}

func defaultMutagenSessionName(slug string) string {
	s := strings.TrimSpace(slug)
	if s == "" {
		s = "project"
	}
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	if len(s) > 30 {
		s = s[:30]
	}
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "dev-sync-" + s
	}
	return fmt.Sprintf("dev-sync-%s-%s", s, hex.EncodeToString(b))
}

func ensureInitialSyncHealthy(mutagenBin, sessionName, destination string) error {
	fmt.Println("🔄 Verificando sincronización inicial...")
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return fmt.Errorf("no se pudo resolver destino remoto de mutagen")
	}

	maxAttempts := 3
	for i := 1; i <= maxAttempts; i++ {
		flush := exec.Command(mutagenBin, "sync", "flush", sessionName)
		if out, err := flush.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				fmt.Printf("⚠️  flush intento %d/%d: %s\n", i, maxAttempts, msg)
			}
		} else {
			_ = out
		}

		if err := remoteFilesPresent(target, remotePath, []string{"go.mod", ".air.toml"}); err == nil {
			fmt.Println("✅ Sync healthy: archivos críticos presentes en VM")
			return nil
		} else {
			fmt.Printf("⚠️  Verificación remota intento %d/%d: %v\n", i, maxAttempts, err)
		}
		time.Sleep(time.Duration(i) * time.Second)
	}

	return fmt.Errorf("sync no quedó saludable tras reintentos (faltan archivos críticos en VM)")
}

func remoteFilesPresent(target, remotePath string, files []string) error {
	names := make([]string, 0, len(files))
	for _, f := range files {
		name := strings.TrimSpace(f)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}

	cleanRoot := strings.TrimSpace(remotePath)
	cleanRoot = strings.TrimSuffix(cleanRoot, "/")
	if cleanRoot == "" {
		cleanRoot = "/"
	}

	checks := make([]string, 0, len(names))
	for _, n := range names {
		abs := cleanRoot + "/" + strings.TrimPrefix(n, "/")
		checks = append(checks, fmt.Sprintf("if [ ! -f %q ]; then echo missing:%q; fi", abs, n))
	}
	cmdText := fmt.Sprintf("%s; ls -la %q", strings.Join(checks, "; "), cleanRoot)
	msg, err := runSSHScript(target, cmdText)
	if err != nil {
		if msg != "" {
			return fmt.Errorf("remote check error (%s): %s", cleanRoot, msg)
		}
		return err
	}

	if strings.Contains(msg, "missing:") {
		return fmt.Errorf("archivos críticos faltantes en VM (%s:%s):\n%s", target, cleanRoot, msg)
	}
	return nil
}

func waitForHTTPReady(url string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	lastErr := ""
	lastCode := 0

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			lastCode = resp.StatusCode
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return resp.StatusCode, nil
			}
			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				return resp.StatusCode, fmt.Errorf("endpoint alcanzable pero protegido por autenticación (HTTP %d)", resp.StatusCode)
			}
			lastErr = fmt.Sprintf("status HTTP %d", resp.StatusCode)
		} else {
			lastErr = err.Error()
		}
		time.Sleep(1 * time.Second)
	}

	if lastCode != 0 {
		return lastCode, fmt.Errorf("timeout esperando 2xx/3xx (último status: %d)", lastCode)
	}
	if strings.TrimSpace(lastErr) == "" {
		lastErr = "sin respuesta"
	}
	return 0, fmt.Errorf("timeout esperando respuesta HTTP (%s)", lastErr)
}

func ensureSSHTrustInteractive(destination string) error {
	target, ok := sshTargetFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	check := newSSHCommand("-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", target, "exit")
	checkOut, checkErr := check.CombinedOutput()
	if checkErr == nil {
		return nil
	}
	checkMsg := strings.ToLower(strings.TrimSpace(string(checkOut)))
	if strings.Contains(checkMsg, "permission denied") || strings.Contains(checkMsg, "ssh keys are required") || strings.Contains(checkMsg, "authentication failed") {
		return nil
	}

	fmt.Printf("🔐 Primera conexión SSH a %s\n", target)
	fmt.Print("¿Confiar en la host key y continuar con la sincronización? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" && answer != "s" && answer != "si" {
		return fmt.Errorf("sincronización cancelada por el usuario (host key no confirmada)")
	}

	accept := newSSHCommand("-o", "StrictHostKeyChecking=accept-new", target, "exit")
	acceptOut, acceptErr := accept.CombinedOutput()
	acceptMsg := strings.ToLower(strings.TrimSpace(string(acceptOut)))
	if acceptErr != nil {
		if strings.Contains(acceptMsg, "permanently added") || strings.Contains(acceptMsg, "known hosts") || strings.Contains(acceptMsg, "permission denied") || strings.Contains(acceptMsg, "ssh keys are required") || strings.Contains(acceptMsg, "authentication failed") {
			fmt.Println(strings.TrimSpace(string(acceptOut)))
			fmt.Println("✅ Host SSH confiado y guardado en known_hosts")
			return nil
		}
		if strings.TrimSpace(string(acceptOut)) != "" {
			return fmt.Errorf("no se pudo registrar host key SSH para %s: %s", target, strings.TrimSpace(string(acceptOut)))
		}
		return fmt.Errorf("no se pudo registrar host key SSH para %s: %w", target, acceptErr)
	}
	if strings.TrimSpace(string(acceptOut)) != "" {
		fmt.Println(strings.TrimSpace(string(acceptOut)))
	}
	fmt.Println("✅ Host SSH confiado y guardado en known_hosts")
	return nil
}

func sshTargetFromMutagenDestination(destination string) (string, bool) {
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	return target, ok
}

func sshTargetAndPathFromMutagenDestination(destination string) (string, string, bool) {
	d := strings.TrimSpace(destination)
	if d == "" {
		return "", "", false
	}
	if strings.HasPrefix(strings.ToLower(d), "ssh://") {
		u, err := url.Parse(d)
		if err != nil || u.Host == "" {
			return "", "", false
		}
		host := u.Hostname()
		if host == "" {
			return "", "", false
		}
		user := ""
		if u.User != nil {
			user = u.User.Username()
		}
		remotePath := u.Path
		if strings.TrimSpace(remotePath) == "" {
			remotePath = "/"
		}
		if user != "" {
			return user + "@" + host, remotePath, true
		}
		return host, remotePath, true
	}
	if strings.Contains(d, "@") && strings.Contains(d, ":") {
		parts := strings.SplitN(d, ":", 2)
		target := strings.TrimSpace(parts[0])
		remotePath := strings.TrimSpace(parts[1])
		if target == "" {
			return "", "", false
		}
		if remotePath == "" {
			remotePath = "/"
		}
		return target, remotePath, true
	}
	return "", "", false
}

func preflightSSHConnection(destination string) error {
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	check := newSSHCommand("-o", "BatchMode=yes", target, "echo", "ok")
	if out, err := check.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		low := strings.ToLower(msg)
		if strings.Contains(low, "please complete registration by running: ssh exe.dev") {
			return fmt.Errorf("SSH no disponible para %s: falta completar onboarding de exe.dev. Ejecuta 'ssh exe.dev' y reintenta", target)
		}
		if strings.Contains(low, "unprotected private key file") || strings.Contains(low, "bad permissions") || strings.Contains(low, "this private key will be ignored") {
			if keyPath, kerr := projectSSHPrivateKeyPath(); kerr == nil {
				if fixErr := enforceSSHPrivateKeyPermissions(keyPath); fixErr == nil {
					retry := newSSHCommand("-o", "BatchMode=yes", target, "echo", "ok")
					if retryOut, retryErr := retry.CombinedOutput(); retryErr == nil {
						fmt.Printf("✅ Permisos SSH corregidos automáticamente: %s\n", keyPath)
						fmt.Printf("✅ SSH operativo: %s\n", target)
						return nil
					} else {
						msg = strings.TrimSpace(string(retryOut))
						low = strings.ToLower(msg)
					}
				}
				return fmt.Errorf("SSH no disponible para %s: permisos inseguros en clave privada (%s). Revisa ACLs del archivo (quita Authenticated Users/Users y deja solo tu usuario con lectura).", target, keyPath)
			}
		}
		if strings.Contains(low, "permission denied") || strings.Contains(low, "publickey") || strings.Contains(low, "ssh keys are required") || strings.Contains(low, "authentication failed") {
			if pubPath, pubKey, keyErr := readLocalSSHPublicKey(); keyErr == nil && strings.TrimSpace(pubKey) != "" {
				return fmt.Errorf("SSH no disponible para %s: autenticación por clave fallida. Sube esta clave pública a exe.dev (%s): %s", target, pubPath, pubKey)
			}
			if pubPath, _, keyErr := readLocalSSHPublicKey(); keyErr == nil {
				return fmt.Errorf("SSH no disponible para %s: autenticación por clave fallida. Sube tu clave pública a exe.dev (%s)", target, pubPath)
			}
			return fmt.Errorf("SSH no disponible para %s: autenticación por clave fallida. Genera una clave con 'ssh-keygen -t ed25519' y súbela a exe.dev", target)
		}
		if msg != "" {
			return fmt.Errorf("SSH no disponible para %s: %s", target, msg)
		}
		return fmt.Errorf("SSH no disponible para %s: %w", target, err)
	}
	fmt.Printf("✅ SSH operativo: %s\n", target)
	return nil
}

func directVMSSHReady(destination string) (bool, string) {
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return false, "destino mutagen inválido"
	}
	check := newSSHCommand(
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		target, "echo", "ok",
	)
	out, err := check.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err == nil {
		return true, ""
	}
	low := strings.ToLower(msg)
	if strings.Contains(low, "please complete registration by running: ssh exe.dev") {
		return false, "registro exe.dev requerido"
	}
	if strings.Contains(low, "permission denied") || strings.Contains(low, "publickey") || strings.Contains(low, "authentication failed") {
		return false, "auth por clave fallida"
	}
	if msg != "" {
		return false, msg
	}
	return false, err.Error()
}

func ensureExeDevSSHOnboarding(destination string) error {
	if skipSSHOnboarding {
		return nil
	}
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	lowTarget := strings.ToLower(strings.TrimSpace(target))
	if !strings.Contains(lowTarget, ".exe.xyz") && !strings.Contains(lowTarget, "exe.dev") {
		return nil
	}

	if hasLocalProjectSSHKey() {
		fmt.Println("✅ Clave SSH local del proyecto detectada; se omite onboarding de exe.dev")
		return nil
	}

	// Si el SSH directo a la VM ya funciona (ej: clave inline del backend),
	// no forzamos onboarding en exe.dev.
	if ok, msg := directVMSSHReady(destination); ok {
		fmt.Println("✅ SSH directo a VM operativo; se omite onboarding de exe.dev")
		return nil
	} else if strings.TrimSpace(msg) != "" {
		fmt.Printf("ℹ️  SSH directo aún no listo (%s). Se evaluará onboarding exe.dev...\n", msg)
	}

	required, msg, err := exeDevRegistrationRequired()
	if err != nil && !required {
		return nil
	}
	if !required {
		return nil
	}

	fmt.Println("🔑 Se requiere onboarding SSH en exe.dev. Ejecutando 'ssh exe.dev'...")
	if strings.TrimSpace(msg) != "" {
		fmt.Println(strings.TrimSpace(msg))
	}
	if !isInteractiveTerminal() {
		return fmt.Errorf("falta completar onboarding SSH en exe.dev (modo no interactivo). Ejecuta 'ssh exe.dev' y reintenta")
	}

	cmd := newSSHCommand("exe.dev")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("onboarding SSH de exe.dev no completado: %w", err)
	}

	if requiredAgain, msgAgain, _ := exeDevRegistrationRequired(); requiredAgain {
		if strings.TrimSpace(msgAgain) != "" {
			return fmt.Errorf("onboarding SSH de exe.dev no completado: %s", strings.TrimSpace(msgAgain))
		}
		return fmt.Errorf("onboarding SSH de exe.dev no completado; ejecuta 'ssh exe.dev' y reintenta")
	}
	fmt.Println("✅ Onboarding SSH de exe.dev completado")
	return nil
}

func exeDevRegistrationRequired() (bool, string, error) {
	check := newSSHCommand("-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "exe.dev", "exit")
	out, err := check.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	low := strings.ToLower(msg)
	if strings.Contains(low, "please complete registration by running: ssh exe.dev") {
		return true, msg, nil
	}
	return false, msg, err
}

func isInteractiveTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func ensureLocalSSHKeySetup() error {
	privPath, pubPath, err := localSSHKeyPaths()
	if err != nil {
		return err
	}
	sshDir := filepath.Dir(privPath)
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(sshDir, 0o700)

	if _, err := os.Stat(privPath); os.IsNotExist(err) {
		if _, lookErr := exec.LookPath("ssh-keygen"); lookErr != nil {
			return fmt.Errorf("falta ssh-keygen para generar %s", privPath)
		}
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", privPath, "-C", "einarc")
		if out, genErr := cmd.CombinedOutput(); genErr != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("no se pudo generar clave SSH: %s", msg)
			}
			return fmt.Errorf("no se pudo generar clave SSH: %w", genErr)
		}
		fmt.Printf("✅ Clave SSH ed25519 generada: %s\n", privPath)
	}

	if err := os.Chmod(privPath, 0o600); err != nil {
		return fmt.Errorf("no se pudo ajustar permisos de %s: %w", privPath, err)
	}
	if err := enforceSSHPrivateKeyPermissions(privPath); err != nil {
		fmt.Printf("⚠️  No se pudieron endurecer permisos de clave SSH (%s): %v\n", privPath, err)
	}

	if _, err := os.Stat(pubPath); os.IsNotExist(err) {
		if _, lookErr := exec.LookPath("ssh-keygen"); lookErr != nil {
			return fmt.Errorf("falta ssh-keygen para derivar %s", pubPath)
		}
		cmd := exec.Command("ssh-keygen", "-y", "-f", privPath)
		out, pubErr := cmd.CombinedOutput()
		if pubErr != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("no se pudo derivar clave pública: %s", msg)
			}
			return fmt.Errorf("no se pudo derivar clave pública: %w", pubErr)
		}
		pubKey := strings.TrimSpace(string(out))
		if pubKey == "" {
			return fmt.Errorf("ssh-keygen devolvió clave pública vacía")
		}
		if err := os.WriteFile(pubPath, []byte(pubKey+"\n"), 0o644); err != nil {
			return fmt.Errorf("no se pudo escribir %s: %w", pubPath, err)
		}
	}
	if err := os.Chmod(pubPath, 0o644); err != nil {
		return fmt.Errorf("no se pudo ajustar permisos de %s: %w", pubPath, err)
	}

	if strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")) != "" {
		if _, err := exec.LookPath("ssh-add"); err == nil {
			cmd := exec.Command("ssh-add", privPath)
			_ = cmd.Run()
		}
	}

	fmt.Printf("✅ Permisos SSH local verificados (%s)\n", privPath)
	return nil
}

func localSSHKeyPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	priv := filepath.Join(home, ".ssh", "id_ed25519")
	pub := priv + ".pub"
	return priv, pub, nil
}

func hasLocalProjectSSHKey() bool {
	keyPath, err := projectSSHPrivateKeyPath()
	if err != nil {
		return false
	}
	st, err := os.Stat(keyPath)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return false
	}
	if err := enforceSSHPrivateKeyPermissions(keyPath); err != nil {
		fmt.Printf("⚠️  No se pudieron endurecer permisos de clave SSH del proyecto (%s): %v\n", keyPath, err)
	}
	return true
}

func projectSSHPrivateKeyPath() (string, error) {
	path, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "id_ed25519"), nil
}

func enforceSSHPrivateKeyPermissions(keyPath string) error {
	if strings.TrimSpace(keyPath) == "" {
		return fmt.Errorf("ruta de clave SSH vacía")
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(keyPath, 0o600)
	}
	currentUser := strings.TrimSpace(os.Getenv("USERNAME"))
	if currentUser == "" {
		currentUser = os.Getenv("USER")
	}
	if currentUser == "" {
		return fmt.Errorf("no se pudo resolver USERNAME en Windows")
	}
	commands := [][]string{
		{"/inheritance:r"},
		{"/remove", "NT AUTHORITY\\Authenticated Users"},
		{"/remove", "BUILTIN\\Users"},
		{"/remove", "Everyone"},
		{"/grant:r", currentUser + ":(R)"},
	}
	for _, args := range commands {
		fullArgs := append([]string{keyPath}, args...)
		cmd := exec.Command("icacls", fullArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("icacls %v falló: %s", args, msg)
			}
			return fmt.Errorf("icacls %v falló: %w", args, err)
		}
	}
	return nil
}

func readLocalSSHPublicKey() (string, string, error) {
	_, pub, err := localSSHKeyPaths()
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(pub)
	if err != nil {
		return pub, "", err
	}
	return pub, strings.TrimSpace(string(b)), nil
}

func ensureRemoteMutagenRoot(destination string) error {
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" || remotePath == "/" {
		return nil
	}
	msg, err := runSSHScript(target, fmt.Sprintf("mkdir -p %q", remotePath))
	if err != nil {
		if msg != "" {
			return fmt.Errorf("no se pudo crear carpeta remota %s en %s: %s", remotePath, target, msg)
		}
		return fmt.Errorf("no se pudo crear carpeta remota %s en %s: %w", remotePath, target, err)
	}
	fmt.Printf("✅ Carpeta remota lista: %s (%s)\n", remotePath, target)
	return nil
}

func normalizeMutagenDestinationForProject(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return v
	}

	// Soporte ssh://user@host/path -> user@host:/path
	if strings.HasPrefix(strings.ToLower(v), "ssh://") {
		u, err := url.Parse(v)
		if err != nil || u.Host == "" {
			return v
		}
		host := u.Hostname()
		if host == "" {
			return v
		}
		user := ""
		if u.User != nil {
			user = u.User.Username()
		}
		remotePath := u.EscapedPath()
		if remotePath == "" {
			remotePath = "/"
		}
		user, remotePath = normalizeExeDevRemoteTarget(user, remotePath)
		if user != "" {
			return fmt.Sprintf("%s@%s:%s", user, host, remotePath)
		}
		return fmt.Sprintf("%s:%s", host, remotePath)
	}

	// Soporte user@host:/path (SCP-style)
	if strings.Contains(v, "@") && strings.Contains(v, ":") {
		parts := strings.SplitN(v, ":", 2)
		left := strings.TrimSpace(parts[0])
		remotePath := strings.TrimSpace(parts[1])
		if left == "" || remotePath == "" {
			return v
		}
		user := ""
		host := left
		if at := strings.Index(left, "@"); at >= 0 {
			user = strings.TrimSpace(left[:at])
			host = strings.TrimSpace(left[at+1:])
		}
		if host == "" {
			return v
		}
		user, remotePath = normalizeExeDevRemoteTarget(user, remotePath)
		if user != "" {
			return fmt.Sprintf("%s@%s:%s", user, host, remotePath)
		}
		return fmt.Sprintf("%s:%s", host, remotePath)
	}

	return v
}

func normalizeExeDevRemoteTarget(user, remotePath string) (string, string) {
	p := strings.TrimSpace(remotePath)
	if p == "" {
		return user, remotePath
	}
	// Estandarizamos proyectos en /home/exedev/workspace/<slug> cuando backend devuelve /app/projects/<slug>
	if strings.HasPrefix(p, "/app/projects/") {
		slug := strings.TrimPrefix(p, "/app/projects/")
		slug = strings.Trim(slug, "/")
		if slug == "" {
			return "exedev", "/home/exedev/workspace"
		}
		return "exedev", "/home/exedev/workspace/" + slug
	}
	return user, remotePath
}

func resolveMutagenBinary() (string, error) {
	if p, err := exec.LookPath("mutagen"); err == nil {
		return p, nil
	}
	if p, _ := localMutagenPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("mutagen no está disponible")
}

func localMutagenPath() (string, error) {
	binName := "mutagen"
	if runtime.GOOS == "windows" {
		binName = "mutagen.exe"
	}
	// Binario compartido junto al CLI (no por proyecto)
	exePath, err := os.Executable()
	if err == nil && strings.TrimSpace(exePath) != "" {
		exeDir := filepath.Dir(exePath)
		if strings.TrimSpace(exeDir) != "" {
			return filepath.Join(exeDir, ".einar", "bin", binName), nil
		}
	}

	// Fallback defensivo al esquema local si no se puede resolver el ejecutable.
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), "bin", binName), nil
}

func downloadMutagenLocal() error {
	url, err := resolveLatestMutagenAssetURL()
	if err != nil {
		return err
	}
	mutagenPath, err := localMutagenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(mutagenPath), 0o700); err != nil {
		return err
	}
	archiveName := strings.TrimSpace(filepath.Base(strings.Split(url, "?")[0]))
	if archiveName == "" {
		archiveName = "mutagen-archive"
	}
	tmpArchive := filepath.Join(filepath.Dir(mutagenPath), archiveName)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("descarga mutagen falló: %s", resp.Status)
	}

	f, err := os.Create(tmpArchive)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	defer os.Remove(tmpArchive)

	baseDir := filepath.Dir(mutagenPath)
	foundExe, foundAgentBundle, err := extractMutagenArchive(tmpArchive, baseDir)
	if err != nil {
		return err
	}

	if !foundExe {
		return fmt.Errorf("no se encontró binario mutagen en el archivo descargado")
	}
	if !foundAgentBundle {
		return fmt.Errorf("no se encontró bundle de agentes en el archivo descargado")
	}

	fmt.Printf("✅ Mutagen descargado en %s\n", mutagenPath)
	return nil
}

func extractMutagenArchive(archivePath, baseDir string) (bool, bool, error) {
	lower := strings.ToLower(strings.TrimSpace(archivePath))
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractMutagenZip(archivePath, baseDir)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractMutagenTarGz(archivePath, baseDir)
	default:
		return false, false, fmt.Errorf("formato de archivo no soportado: %s", archivePath)
	}
}

func extractMutagenZip(archivePath, baseDir string) (bool, bool, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return false, false, err
	}
	defer zr.Close()

	foundExe := false
	foundAgentBundle := false

	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(filepath.Base(zf.Name))
		isMutagenBin := name == "mutagen.exe" || name == "mutagen"
		if !isMutagenBin && !strings.Contains(name, "agent") {
			continue
		}

		rc, err := zf.Open()
		if err != nil {
			return false, false, err
		}

		dst := filepath.Join(baseDir, filepath.Base(zf.Name))
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return false, false, err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return false, false, err
		}
		out.Close()
		rc.Close()

		if isMutagenBin {
			foundExe = true
		}
		if strings.Contains(name, "agent") {
			foundAgentBundle = true
		}
	}

	return foundExe, foundAgentBundle, nil
}

func extractMutagenTarGz(archivePath, baseDir string) (bool, bool, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return false, false, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return false, false, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	foundExe := false
	foundAgentBundle := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, false, err
		}
		if hdr == nil || hdr.FileInfo().IsDir() {
			continue
		}

		name := strings.ToLower(filepath.Base(hdr.Name))
		isMutagenBin := name == "mutagen.exe" || name == "mutagen"
		if !isMutagenBin && !strings.Contains(name, "agent") {
			continue
		}

		dst := filepath.Join(baseDir, filepath.Base(hdr.Name))
		mode := os.FileMode(0o644)
		if isMutagenBin {
			mode = 0o755
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return false, false, err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return false, false, err
		}
		if err := out.Close(); err != nil {
			return false, false, err
		}

		if isMutagenBin {
			foundExe = true
		}
		if strings.Contains(name, "agent") {
			foundAgentBundle = true
		}
	}

	return foundExe, foundAgentBundle, nil
}

func resolveLatestMutagenAssetURL() (string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/mutagen-io/mutagen/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github latest release request falló: %s body=%s", resp.Status, strings.TrimSpace(string(b)))
	}

	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}
	var payload struct {
		Assets []asset `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	osName := runtime.GOOS
	arch := runtime.GOARCH
	osAliases := []string{osName}
	if osName == "darwin" {
		osAliases = append(osAliases, "macos", "osx")
	}
	archAliases := []string{arch}
	if arch == "amd64" {
		archAliases = append(archAliases, "x86_64", "x64")
	}
	if arch == "arm64" {
		archAliases = append(archAliases, "aarch64")
	}
	for _, a := range payload.Assets {
		name := strings.ToLower(strings.TrimSpace(a.Name))
		if !strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tgz") {
			continue
		}
		osMatch := false
		for _, oa := range osAliases {
			if strings.Contains(name, oa) {
				osMatch = true
				break
			}
		}
		if !osMatch {
			continue
		}
		archMatch := false
		for _, aa := range archAliases {
			if strings.Contains(name, aa) {
				archMatch = true
				break
			}
		}
		if !archMatch {
			continue
		}
		if strings.TrimSpace(a.URL) != "" {
			return strings.TrimSpace(a.URL), nil
		}
	}

	return "", fmt.Errorf("no se encontró asset mutagen para %s/%s en latest release", osName, arch)
}

func init() {
	initCmd.Flags().BoolVar(&skipMutagenCheck, "skip-mutagen-check", false, "No verifica/instala Mutagen automáticamente")
	initCmd.Flags().StringVar(&initMutagenDestination, "mutagen-destination", "", "Destino remoto Mutagen (ej: docker://mi-contenedor/var/www)")
	initCmd.Flags().StringVar(&initMutagenName, "mutagen-name", "", "Nombre de sesión en mutagen.yml (por defecto: dev-sync-<slug>-<shortid>)")
	initCmd.Flags().BoolVar(&skipMutagenStart, "skip-mutagen-start", false, "No ejecuta 'mutagen project start'")
	initCmd.Flags().BoolVar(&skipSSHOnboarding, "skip-ssh-onboarding", false, "No ejecuta onboarding automático con 'ssh exe.dev'")
	initCmd.Flags().BoolVar(&skipAirAutoStart, "skip-air-auto-start", false, "No inicia Air automáticamente en la VM")
	initCmd.Flags().BoolVar(&skipInitialCommit, "skip-initial-commit", false, "No crea commit inicial del scaffold del proyecto")
	initCmd.Flags().BoolVar(&forceWorkspaceTakeover, "force-workspace-takeover", false, "Toma control del workspace global aunque otro owner lo tenga reservado")
	rootCmd.AddCommand(initCmd)
}
