package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/config"

	"github.com/spf13/cobra"
)

var (
	casdoorOriginFlag string
	oidcClientIDFlag  string
	oidcProviderFlag  string
)

var loginCmd = &cobra.Command{
	Use:   "login [email]",
	Short: "Login en Einar (browser + Casdoor) o guarda token manual",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		loginEmail, err := parseLoginEmailArg(args)
		if err != nil {
			return err
		}

		manual := strings.TrimSpace(tokenFlag)
		if manual != "" {
			if loginEmail != "" {
				return fmt.Errorf("no puedes combinar --token con email posicional")
			}
			return saveManualToken(manual)
		}

		cfg, err := resolveLoginConfig()
		if err != nil {
			return err
		}

		casdoorOrigin := resolvedCasdoorOrigin(cfg.APIURL)
		clientID := resolvedClientID()

		idToken, refreshToken, err := runDeviceLogin(casdoorOrigin, clientID, loginEmail)
		if err != nil {
			return err
		}

		cfg.Token = idToken
		cfg.RefreshToken = refreshToken
		if err := config.SaveGlobal(cfg); err != nil {
			return err
		}
		if err := ensureGitIgnoreHasEinar(); err != nil {
			return err
		}
		path, _ := config.GlobalConfigPath()
		fmt.Printf("✅ Login completado en %s\n", path)
		fmt.Printf("Token: %s\n", config.MaskToken(idToken))
		return nil
	},
}

func parseLoginEmailArg(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	email := strings.TrimSpace(args[0])
	if email == "" {
		return "", nil
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(strings.TrimSpace(parsed.Address), email) {
		return "", fmt.Errorf("email inválido: %q", email)
	}
	return parsed.Address, nil
}

func saveManualToken(token string) error {
	cfg, err := resolveLoginConfig()
	if err != nil {
		return err
	}
	cfg.Token = token
	cfg.RefreshToken = ""
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	if err := ensureGitIgnoreHasEinar(); err != nil {
		return err
	}
	path, _ := config.GlobalConfigPath()
	fmt.Printf("✅ Login guardado en %s\n", path)
	fmt.Printf("Token: %s\n", config.MaskToken(token))
	return nil
}

func resolveLoginConfig() (config.Config, error) {
	cfg, err := config.Resolve(apiURLFlag, "")
	if err == nil {
		return cfg, nil
	}
	return config.Config{APIURL: "https://einar.exe.xyz"}, nil
}

func resolvedCasdoorOrigin(apiURL string) string {
	casdoorOrigin := strings.TrimSpace(casdoorOriginFlag)
	if casdoorOrigin == "" {
		casdoorOrigin = strings.TrimSpace(os.Getenv("EINAR_CASDOOR_ORIGIN"))
	}
	if casdoorOrigin == "" {
		casdoorOrigin = deriveCasdoorOrigin(apiURL)
	}
	return strings.TrimRight(casdoorOrigin, "/")
}

func resolvedClientID() string {
	clientID := strings.TrimSpace(oidcClientIDFlag)
	if clientID == "" {
		clientID = strings.TrimSpace(os.Getenv("EINAR_OIDC_CLIENT_ID"))
	}
	if clientID == "" {
		clientID = "da89744f1f42e0516f2c"
	}
	return clientID
}

func tryRefreshAndSave(cfg *config.Config) (bool, error) {
	if strings.TrimSpace(cfg.RefreshToken) == "" {
		return false, nil
	}
	casdoorOrigin := resolvedCasdoorOrigin(cfg.APIURL)
	clientID := resolvedClientID()
	idToken, refreshToken, err := refreshTokens(context.Background(), casdoorOrigin, clientID, cfg.RefreshToken)
	if err != nil {
		return false, nil // refresh falló: caller decide pedir re-login
	}
	cfg.Token = idToken
	if refreshToken != "" {
		cfg.RefreshToken = refreshToken
	}
	if err := config.SaveGlobal(*cfg); err != nil {
		return false, err
	}
	return true, nil
}

func runPKCELogin(casdoorOrigin, clientID, provider string) (string, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", fmt.Errorf("no se pudo abrir callback local: %w", err)
	}
	defer ln.Close()

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
	state, err := randomURLSafe(32)
	if err != nil {
		return "", "", err
	}
	verifier, err := randomURLSafe(64)
	if err != nil {
		return "", "", err
	}
	challenge := pkceChallenge(verifier)

	authURL, err := buildAuthorizeURL(casdoorOrigin, clientID, redirectURI, state, challenge, provider)
	if err != nil {
		return "", "", err
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if e := r.URL.Query().Get("error"); e != "" {
			errCh <- fmt.Errorf("oauth error: %s (%s)", e, r.URL.Query().Get("error_description"))
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Login cancelado. Puedes cerrar esta ventana."))
			return
		}
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state inválido en callback")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("State inválido. Cierra esta ventana."))
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			errCh <- fmt.Errorf("callback sin code")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Callback sin code. Cierra esta ventana."))
			return
		}
		_, _ = w.Write([]byte("Login exitoso ✅ Puedes volver a la terminal."))
		codeCh <- code
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	fmt.Println("Abriendo navegador para login...")
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("No se pudo abrir automáticamente. Abre esta URL manualmente:\n%s\n", authURL)
	} else {
		fmt.Printf("Si no se abrió, visita:\n%s\n", authURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var code string
	select {
	case <-ctx.Done():
		return "", "", fmt.Errorf("timeout esperando callback de login")
	case e := <-errCh:
		return "", "", e
	case code = <-codeCh:
	}

	return exchangeCodeForTokens(ctx, casdoorOrigin, clientID, code, verifier, redirectURI)
}

func runDeviceLogin(casdoorOrigin, clientID, loginEmail string) (string, string, error) {
	codeEndpoint := strings.TrimRight(casdoorOrigin, "/") + "/api/auth/device/code"
	tokenEndpoint := strings.TrimRight(casdoorOrigin, "/") + "/api/auth/device/token"

	payload, err := json.Marshal(map[string]string{"client_id": clientID})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, codeEndpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("device code request falló: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", "", fmt.Errorf("device code %d: %s", resp.StatusCode, string(raw))
	}

	var device struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	if err := json.Unmarshal(raw, &device); err != nil {
		return "", "", fmt.Errorf("respuesta inválida de device code: %w", err)
	}
	if strings.TrimSpace(device.DeviceCode) == "" {
		return "", "", fmt.Errorf("device code vacío")
	}

	verificationURL := strings.TrimSpace(device.VerificationURIComplete)
	if verificationURL == "" {
		verificationURL = strings.TrimSpace(device.VerificationURI)
	}
	if verificationURL == "" {
		return "", "", fmt.Errorf("verification_uri ausente")
	}
	verificationURL = appendLoginHint(verificationURL, loginEmail)

	fmt.Printf("\n🔐 Login por código de dispositivo\n")
	if loginEmail != "" {
		fmt.Printf("Cuenta sugerida: %s\n", loginEmail)
	}
	if device.UserCode != "" {
		fmt.Printf("Código: %s\n", device.UserCode)
	}
	fmt.Printf("Abrí esta URL en tu navegador:\n%s\n\n", verificationURL)
	if err := openBrowser(verificationURL); err == nil {
		fmt.Println("Intentando abrir navegador automáticamente...")
	}

	interval := device.Interval
	if interval <= 0 {
		interval = 5
	}
	expiresIn := device.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 900
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("timeout esperando autorización de dispositivo")
		}

		pollBody, _ := json.Marshal(map[string]string{
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
			"device_code": device.DeviceCode,
			"client_id":   clientID,
		})
		pollReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenEndpoint, bytes.NewReader(pollBody))
		if err != nil {
			return "", "", err
		}
		pollReq.Header.Set("Content-Type", "application/json")

		pollResp, err := (&http.Client{Timeout: 20 * time.Second}).Do(pollReq)
		if err != nil {
			return "", "", fmt.Errorf("polling device token falló: %w", err)
		}
		pollRaw, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var token struct {
			AccessToken  string `json:"access_token"`
			IDToken      string `json:"id_token"`
			RefreshToken string `json:"refresh_token"`
			Error        string `json:"error"`
		}
		_ = json.Unmarshal(pollRaw, &token)

		switch strings.TrimSpace(token.Error) {
		case "":
			id := strings.TrimSpace(token.IDToken)
			if id == "" {
				id = strings.TrimSpace(token.AccessToken)
			}
			if id == "" {
				return "", "", fmt.Errorf("token response sin id_token/access_token: %s", string(pollRaw))
			}
			return id, strings.TrimSpace(token.RefreshToken), nil
		case "authorization_pending":
			// seguir esperando
		case "slow_down":
			interval += 5
		default:
			if pollResp.StatusCode >= 200 && pollResp.StatusCode <= 299 {
				return "", "", fmt.Errorf("device flow error: %s", token.Error)
			}
			return "", "", fmt.Errorf("device token %d: %s", pollResp.StatusCode, string(pollRaw))
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func appendLoginHint(rawURL, loginEmail string) string {
	if strings.TrimSpace(loginEmail) == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set("login_hint", strings.TrimSpace(loginEmail))
	u.RawQuery = q.Encode()
	return u.String()
}

func buildAuthorizeURL(casdoorOrigin, clientID, redirectURI, state, challenge, provider string) (string, error) {
	u, err := url.Parse(casdoorOrigin + "/login/oauth/authorize")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "openid profile email")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	if provider != "" {
		q.Set("provider", provider)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func exchangeCodeForTokens(ctx context.Context, casdoorOrigin, clientID, code, verifier, redirectURI string) (string, string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)
	return tokenRequest(ctx, casdoorOrigin, form)
}

func refreshTokens(ctx context.Context, casdoorOrigin, clientID, refreshToken string) (string, string, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	form.Set("refresh_token", refreshToken)
	return tokenRequest(ctx, casdoorOrigin, form)
}

func tokenRequest(ctx context.Context, casdoorOrigin string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, casdoorOrigin+"/api/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("token exchange falló: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", "", fmt.Errorf("token exchange %d: %s", resp.StatusCode, string(raw))
	}

	var payload struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("respuesta inválida de token endpoint: %w", err)
	}
	if strings.TrimSpace(payload.IDToken) == "" {
		return "", "", fmt.Errorf("Casdoor no devolvió id_token")
	}
	return strings.TrimSpace(payload.IDToken), strings.TrimSpace(payload.RefreshToken), nil
}

func deriveCasdoorOrigin(apiURL string) string {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "https://einar.exe.xyz"
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "localhost" || host == "127.0.0.1" {
		scheme := u.Scheme
		if scheme == "" {
			scheme = "http"
		}
		return scheme + "://" + host + ":8000"
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	return u.Scheme + "://" + u.Host
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func randomURLSafe(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("no se pudo generar random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func ensureGitIgnoreHasEinar() error {
	const entry = ".einar/"
	path := ".gitignore"

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(entry+"\n"), 0o644)
		}
		return err
	}

	lines := strings.Split(string(b), "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) == entry {
			return nil
		}
	}

	appendText := "\n" + entry + "\n"
	if len(b) == 0 || strings.HasSuffix(string(b), "\n") {
		appendText = entry + "\n"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(appendText)
	return err
}

func init() {
	loginCmd.Flags().StringVar(&casdoorOriginFlag, "casdoor-origin", "", "Origen de Casdoor (fallback: EINAR_CASDOOR_ORIGIN o derivado de --api-url)")
	loginCmd.Flags().StringVar(&oidcClientIDFlag, "oidc-client-id", "", "OIDC client_id para PKCE (fallback: EINAR_OIDC_CLIENT_ID, default: da89744f1f42e0516f2c)")
	loginCmd.Flags().StringVar(&oidcProviderFlag, "provider", "provider_google_einar", "Provider Casdoor opcional para saltar UI (vacío para no enviar)")
	rootCmd.AddCommand(loginCmd)
}
