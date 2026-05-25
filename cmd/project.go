package cmd

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/rand"
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

	"einarc/internal/api"
	"einarc/internal/config"

	"github.com/spf13/cobra"
)

var (
	skipMutagenCheck      bool
	initMutagenDestination string
	initMutagenName        string
	skipMutagenStart       bool
	skipAirAutoStart       bool
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

		if err := ensureGoProjectScaffold(strings.TrimSpace(resp.Slug)); err != nil {
			return fmt.Errorf("no se pudo preparar scaffold Go base: %w", err)
		}
		if err := ensureLocalAirConfig(); err != nil {
			return fmt.Errorf("no se pudo preparar .air.toml base: %w", err)
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

		airStarted := false
		mutagenReady := false
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

		if strings.TrimSpace(cfg.LastVMHTTPSURL) != "" {
			httpChecked = true
			if airStarted {
				fmt.Printf("🌐 Verificando endpoint HTTP: %s\n", strings.TrimSpace(cfg.LastVMHTTPSURL))
				if code, err := waitForHTTPReady(strings.TrimSpace(cfg.LastVMHTTPSURL), 45*time.Second); err != nil {
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

func ensureGoProjectScaffold(slug string) error {
	s := strings.TrimSpace(slug)
	if s == "" {
		s = "app"
	}

	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		goMod := fmt.Sprintf("module %s\n\ngo 1.22\n", s)
		if werr := os.WriteFile("go.mod", []byte(goMod), 0o644); werr != nil {
			return werr
		}
		fmt.Println("✅ go.mod generado")
	}

	if _, err := os.Stat("main.go"); os.IsNotExist(err) {
		mainGo := `package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok: %s\\n", r.URL.Path)
	})

	addr := ":" + port
	log.Printf("server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
`
		if werr := os.WriteFile("main.go", []byte(mainGo), 0o644); werr != nil {
			return werr
		}
		fmt.Println("✅ main.go generado")
	}

	return nil
}

func airTomlContent() string {
	return `root = "."

tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/main ."
  bin = "./tmp/main"
  include_ext = ["go", "tpl", "tmpl", "html"]
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

func setupAndStartRemoteAir(cfg *config.Config) error {
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

	installCmd := `if ! command -v go >/dev/null 2>&1; then echo "missing go"; exit 21; fi; if ! command -v air >/dev/null 2>&1 && [ ! -x "$HOME/go/bin/air" ]; then go install github.com/air-verse/air@latest; fi; if command -v air >/dev/null 2>&1; then echo "air ready: $(command -v air)"; elif [ -x "$HOME/go/bin/air" ]; then echo "air ready: $HOME/go/bin/air"; else echo "missing air after install"; exit 22; fi`
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

	remoteEnsureAirToml := fmt.Sprintf("cd %q && if [ ! -f .air.toml ]; then cat > .air.toml <<'EOF'\n%s\nEOF\n echo 'generated .air.toml remotely'; fi", remotePath, airTomlContent())
	msg, err = runSSHScriptWithTimeout(target, remoteEnsureAirToml, 45*time.Second)
	if err != nil {
		if msg != "" {
			return fmt.Errorf("falló preparación remota de .air.toml: %s", msg)
		}
		return fmt.Errorf("falló preparación remota de .air.toml: %w", err)
	}

	startCmd := fmt.Sprintf(`cd %q && mkdir -p tmp && AIR_BIN="$(command -v air || echo $HOME/go/bin/air)" && if [ ! -f .air.toml ]; then echo "missing .air.toml"; exit 12; fi && if [ -f .air.pid ] && kill -0 "$(cat .air.pid)" 2>/dev/null; then echo "air ya corriendo"; exit 0; fi && nohup "$AIR_BIN" -c .air.toml > .air.log 2>&1 & echo $! > .air.pid && sleep 1 && if [ -f .air.pid ] && kill -0 "$(cat .air.pid)" 2>/dev/null; then echo "air started pid=$(cat .air.pid)"; else echo "air exited"; tail -n 120 .air.log 2>/dev/null || true; exit 13; fi`, remotePath)
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
		sessionName = defaultMutagenSessionName(cfg.LastProjectSlug)
		cfg.MutagenSessionName = sessionName
		_ = config.Save(*cfg)
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

	if err := ensureSSHTrustInteractive(destination); err != nil {
		return err
	}
	if err := preflightSSHConnection(destination); err != nil {
		return err
	}
	if err := ensureRemoteMutagenRoot(destination); err != nil {
		return err
	}

	// Evita usar un daemon viejo iniciado con otro binario/carpeta de agentes.
	_ = exec.Command(mutagenBin, "daemon", "stop").Run()

	cmd := exec.Command(mutagenBin, "project", "start")
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		lowOut := strings.ToLower(output)
		if strings.Contains(lowOut, "project already running") {
			fmt.Println("✅ Mutagen ya estaba corriendo para este proyecto")
			if err := ensureInitialSyncHealthy(mutagenBin, sessionName, destination); err != nil {
				fmt.Printf("⚠️  Sync aún no está saludable (continuando): %v\n", err)
			}
			return nil
		}
		if strings.Contains(lowOut, "unable to connect to daemon") || strings.Contains(lowOut, "connection timed out") {
			fmt.Println("ℹ️  Reintentando: iniciando daemon Mutagen y relanzando project start...")
			startDaemon := exec.Command(mutagenBin, "daemon", "start")
			if dOut, dErr := startDaemon.CombinedOutput(); dErr != nil {
				daemonMsg := strings.TrimSpace(string(dOut))
				if daemonMsg != "" {
					fmt.Println(daemonMsg)
				}
				if output != "" {
					fmt.Println(output)
				}
				return dErr
			}
			retry := exec.Command(mutagenBin, "project", "start")
			rOut, rErr := retry.CombinedOutput()
			rMsg := strings.TrimSpace(string(rOut))
			if rErr != nil {
				if rMsg != "" {
					fmt.Println(rMsg)
				}
				if output != "" {
					fmt.Println(output)
				}
				return rErr
			}
			if rMsg != "" {
				fmt.Println(rMsg)
			}
			fmt.Println("✅ Mutagen sync iniciado (mutagen project start)")
			if err := ensureInitialSyncHealthy(mutagenBin, sessionName, destination); err != nil {
				fmt.Printf("⚠️  Sync aún no está saludable (continuando): %v\n", err)
			}
			printMutagenPostInitChecklist(destination, sessionName, strings.TrimSpace(cfg.LastVMHTTPSURL))
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
	if err := ensureInitialSyncHealthy(mutagenBin, sessionName, destination); err != nil {
		fmt.Printf("⚠️  Sync aún no está saludable (continuando): %v\n", err)
	}
	printMutagenPostInitChecklist(destination, sessionName, strings.TrimSpace(cfg.LastVMHTTPSURL))
	return nil
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

		if err := remoteFilesPresent(target, remotePath, []string{"main.go", ".air.toml"}); err == nil {
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
			lastErr = fmt.Sprintf("status HTTP %d", resp.StatusCode)
		} else {
			lastErr = err.Error()
		}
		time.Sleep(2 * time.Second)
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
	check := exec.Command("ssh", "-o", "BatchMode=yes", target, "echo", "ok")
	if out, err := check.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("SSH no disponible para %s: %s", target, msg)
		}
		return fmt.Errorf("SSH no disponible para %s: %w", target, err)
	}
	fmt.Printf("✅ SSH operativo: %s\n", target)
	return nil
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
	initCmd.Flags().StringVar(&initMutagenName, "mutagen-name", "", "Nombre de sesión en mutagen.yml (por defecto: dev-sync-<slug>-<shortid>)")
	initCmd.Flags().BoolVar(&skipMutagenStart, "skip-mutagen-start", false, "No ejecuta 'mutagen project start'")
	initCmd.Flags().BoolVar(&skipAirAutoStart, "skip-air-auto-start", false, "No inicia Air automáticamente en la VM")
	rootCmd.AddCommand(initCmd)
}
