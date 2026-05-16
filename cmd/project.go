package cmd

import (
	"archive/zip"
	"bufio"
	"context"
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

	"einarc/internal/api"
	"einarc/internal/config"

	"github.com/spf13/cobra"
)

var (
	skipMutagenCheck     bool
	initMutagenDestination string
	initMutagenName        string
	skipMutagenStart      bool
)

var initCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Inicializa un proyecto en Einar",
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
			fmt.Printf("[debug] Body: {\"name\":%q}\n", name)
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
		if strings.TrimSpace(resp.ProjectID) == "" || strings.TrimSpace(resp.Slug) == "" {
			return fmt.Errorf("respuesta inválida del backend: projectId/slug vacíos (status=%q)", strings.TrimSpace(resp.Status))
		}

		localProjectDir := strings.TrimSpace(resp.Slug)
		if localProjectDir == "" {
			localProjectDir = strings.TrimSpace(name)
		}
		if err := os.MkdirAll(localProjectDir, 0o755); err != nil {
			return fmt.Errorf("no se pudo crear carpeta local del proyecto: %w", err)
		}
		if err := os.Chdir(localProjectDir); err != nil {
			return fmt.Errorf("no se pudo entrar a la carpeta local del proyecto: %w", err)
		}

		cfg.LastProjectID = strings.TrimSpace(resp.ProjectID)
		cfg.LastProjectSlug = strings.TrimSpace(resp.Slug)
		if strings.TrimSpace(resp.MutagenDestination) != "" {
			cfg.MutagenDestination = strings.TrimSpace(resp.MutagenDestination)
		} else if strings.TrimSpace(resp.VMSshDest) != "" {
			cfg.MutagenDestination = strings.TrimSpace(resp.VMSshDest)
		}
		if strings.TrimSpace(resp.MutagenSessionName) != "" {
			cfg.MutagenSessionName = strings.TrimSpace(resp.MutagenSessionName)
		}
		if strings.TrimSpace(resp.VMName) != "" {
			cfg.LastVMName = strings.TrimSpace(resp.VMName)
		}
		if strings.TrimSpace(resp.VMHTTPSURL) != "" {
			cfg.LastVMHTTPSURL = strings.TrimSpace(resp.VMHTTPSURL)
		}
		if strings.TrimSpace(resp.VMSshDest) != "" {
			cfg.LastVMSshDest = strings.TrimSpace(resp.VMSshDest)
		}
		if strings.TrimSpace(resp.ProjectAPIToken) != "" {
			cfg.ProjectAPIToken = strings.TrimSpace(resp.ProjectAPIToken)
		}
		if strings.TrimSpace(resp.VMSshPrivateKey) != "" {
			if err := writeSSHPrivateKey(resp.VMSshPrivateKey); err != nil {
				return fmt.Errorf("no se pudo guardar clave SSH privada: %w", err)
			}
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("no se pudo guardar config local: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(resp)
		}

		fmt.Println("✅ Proyecto creado")
		fmt.Printf("ID: %s\n", resp.ProjectID)
		fmt.Printf("Name: %s\n", resp.Name)
		fmt.Printf("Slug: %s\n", resp.Slug)
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

		if !skipMutagenCheck {
			if err := ensureMutagenOnWindows(); err != nil {
				fmt.Printf("⚠️  Proyecto creado, pero no se pudo verificar/instalar Mutagen: %v\n", err)
				fmt.Println("   Puedes instalarlo manualmente y luego correr: einarc mutagen ...")
			} else {
				fmt.Println("✅ Mutagen disponible en esta máquina")
				if err := setupAndStartMutagen(&cfg); err != nil {
					fmt.Printf("⚠️  Mutagen disponible, pero no se pudo auto-configurar/start: %v\n", err)
					fmt.Println("   Puedes correr manualmente: einarc mutagen --destination <destino> && mutagen project start")
				}
			}
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
	default:
		return fmt.Sprintf("Error API (%d): %s", ae.StatusCode, ae.Message)
	}
}

func ensureMutagenOnWindows() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if _, err := exec.LookPath("mutagen"); err == nil {
		return nil
	}
	if p, _ := localMutagenPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return nil
		}
	}

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

	// Intento 3: descarga directa (fallback)
	if err := downloadMutagenLocal(); err == nil {
		return nil
	} else {
		fmt.Printf("⚠️  Fallback descarga Mutagen falló: %v\n", err)
	}

	return fmt.Errorf("mutagen no está en PATH y no se pudo instalar automáticamente")
}

func writeSSHPrivateKey(key string) error {
	path, err := config.ConfigPath()
	if err != nil {
		return err
	}
	keyPath := filepath.Join(filepath.Dir(path), "id_ed25519")
	content := strings.TrimSpace(key) + "\n"
	if err := os.WriteFile(keyPath, []byte(content), 0o600); err != nil {
		return err
	}
	fmt.Printf("✅ SSH private key guardada en %s\n", keyPath)
	return nil
}

func setupAndStartMutagen(cfg *config.Config) error {
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
		sessionName = "dev-sync"
	}

	if _, err := os.Stat("mutagen.yml"); os.IsNotExist(err) {
		if destination == "" {
			return fmt.Errorf("falta destino mutagen (usa --mutagen-destination o configura PROJECTS_SYNC_SSH_* en backend)")
		}
		destination = normalizeMutagenDestinationForProject(destination)
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

	if err := ensureSSHTrustInteractive(destination); err != nil {
		return err
	}

	// Evita usar un daemon viejo iniciado con otro binario/carpeta de agentes.
	_ = exec.Command(mutagenBin, "daemon", "stop").Run()

	cmd := exec.Command(mutagenBin, "project", "start")
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if strings.Contains(strings.ToLower(output), "project already running") {
			fmt.Println("✅ Mutagen ya estaba corriendo para este proyecto")
			return nil
		}
		if output != "" {
			fmt.Println(output)
		}
		return err
	}
	if output != "" {
		fmt.Println(output)
	}
	fmt.Println("✅ Mutagen sync iniciado (mutagen project start)")
	return nil
}

func ensureSSHTrustInteractive(destination string) error {
	target, ok := sshTargetFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	check := exec.Command("ssh", "-o", "BatchMode=yes", target, "exit")
	if err := check.Run(); err == nil {
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

	accept := exec.Command("ssh", "-o", "StrictHostKeyChecking=accept-new", target, "exit")
	accept.Stdout = os.Stdout
	accept.Stderr = os.Stderr
	if err := accept.Run(); err != nil {
		return fmt.Errorf("no se pudo registrar host key SSH para %s: %w", target, err)
	}
	fmt.Println("✅ Host SSH confiado y guardado en known_hosts")
	return nil
}

func sshTargetFromMutagenDestination(destination string) (string, bool) {
	d := strings.TrimSpace(destination)
	if d == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(d), "ssh://") {
		u, err := url.Parse(d)
		if err != nil || u.Host == "" {
			return "", false
		}
		host := u.Hostname()
		if host == "" {
			return "", false
		}
		user := ""
		if u.User != nil {
			user = u.User.Username()
		}
		if user != "" {
			return user + "@" + host, true
		}
		return host, true
	}
	if strings.Contains(d, "@") && strings.Contains(d, ":") {
		parts := strings.SplitN(d, ":", 2)
		if strings.TrimSpace(parts[0]) == "" {
			return "", false
		}
		return strings.TrimSpace(parts[0]), true
	}
	return "", false
}

func normalizeMutagenDestinationForProject(raw string) string {
	v := strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(v), "ssh://") {
		return v
	}
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
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if user != "" {
		return fmt.Sprintf("%s@%s:%s", user, host, path)
	}
	return fmt.Sprintf("%s:%s", host, path)
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
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	binName := "mutagen"
	if runtime.GOOS == "windows" {
		binName = "mutagen.exe"
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
	tmpZip := filepath.Join(filepath.Dir(mutagenPath), "mutagen.zip")

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("descarga mutagen falló: %s", resp.Status)
	}

	f, err := os.Create(tmpZip)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	defer os.Remove(tmpZip)

	zr, err := zip.OpenReader(tmpZip)
	if err != nil {
		return err
	}
	defer zr.Close()

	foundExe := false
	foundAgentBundle := false
	baseDir := filepath.Dir(mutagenPath)

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
			return err
		}

		dst := filepath.Join(baseDir, filepath.Base(zf.Name))
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
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

	if !foundExe {
		return fmt.Errorf("no se encontró binario mutagen en el zip descargado")
	}
	if !foundAgentBundle {
		return fmt.Errorf("no se encontró bundle de agentes en el zip descargado")
	}

	fmt.Printf("✅ Mutagen descargado en %s\n", mutagenPath)
	return nil
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
	for _, a := range payload.Assets {
		name := strings.ToLower(strings.TrimSpace(a.Name))
		if !strings.HasSuffix(name, ".zip") {
			continue
		}
		if strings.Contains(name, osName) && strings.Contains(name, arch) {
			if strings.TrimSpace(a.URL) != "" {
				return strings.TrimSpace(a.URL), nil
			}
		}
	}

	return "", fmt.Errorf("no se encontró asset mutagen para %s/%s en latest release", osName, arch)
}

func init() {
	initCmd.Flags().BoolVar(&skipMutagenCheck, "skip-mutagen-check", false, "No verifica/instala Mutagen automáticamente")
	initCmd.Flags().StringVar(&initMutagenDestination, "mutagen-destination", "", "Destino remoto Mutagen (ej: docker://mi-contenedor/var/www)")
	initCmd.Flags().StringVar(&initMutagenName, "mutagen-name", "dev-sync", "Nombre de sesión en mutagen.yml")
	initCmd.Flags().BoolVar(&skipMutagenStart, "skip-mutagen-start", false, "No ejecuta 'mutagen project start'")
	rootCmd.AddCommand(initCmd)
}
