package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Ignaciojeria/sync/internal/config"
	"github.com/spf13/cobra"
)

const (
	topologyHeartbeatInterval = 30 * time.Second
	heartbeatPIDFileName      = "topology-heartbeat.pid"
	heartbeatPIDFileEnv       = "SYNC_TOPOLOGY_HEARTBEAT_PID_FILE"
)

var topologyHeartbeatCmd = &cobra.Command{
	Use:    "topology-heartbeat",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Resolve("", "")
		if err != nil {
			return err
		}
		projectPath, err := os.Getwd()
		if err != nil {
			return err
		}
		monitor := newTopologyHeartbeatMonitor(cfg, topologyHeartbeatOptions{
			ProjectPath:  projectPath,
			WorkspaceURL: cfg.LastVMHTTPSURL,
			ProjectName:  cfg.LastProjectSlug,
			SessionID:    topologySessionIDForConfig(projectPath, cfg),
			ClientName:   topologyClientName(cfg),
			Source:       "mutagen",
			Interval:     topologyHeartbeatInterval,
		})
		return monitor.Run(cmd.Context())
	},
}

type topologyHeartbeatOptions struct {
	ProjectPath  string
	WorkspaceURL string
	ProjectName  string
	SessionID    string
	ClientName   string
	Source       string
	Interval     time.Duration
}

type topologyHeartbeatMonitor struct {
	cfg     config.Config
	http    *http.Client
	opts    topologyHeartbeatOptions
	now     func() time.Time
	tokenFn func(context.Context, config.Config) (string, error)
}

type topologySessionFile struct {
	SessionID   string    `json:"session_id"`
	ProjectName string    `json:"project_name"`
	Email       string    `json:"email,omitempty"`
	Hostname    string    `json:"hostname,omitempty"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	Source      string    `json:"source,omitempty"`
}

func newTopologyHeartbeatMonitor(cfg config.Config, opts topologyHeartbeatOptions) *topologyHeartbeatMonitor {
	if opts.Interval <= 0 {
		opts.Interval = topologyHeartbeatInterval
	}
	cfg = mergeTopologyIdentityConfig(cfg)
	return &topologyHeartbeatMonitor{
		cfg:  cfg,
		http: &http.Client{Timeout: 15 * time.Second},
		opts: opts,
		now:  time.Now,
		tokenFn: func(ctx context.Context, cfg config.Config) (string, error) {
			if source, err := machineTokenSourceFromConfig(cfg); err == nil && source != nil {
				return source.Token(ctx)
			}
			if token := strings.TrimSpace(cfg.Token); token != "" {
				return token, nil
			}
			return "", fmt.Errorf("no hay token ni machine auth para reportar heartbeat")
		},
	}
}

func (m *topologyHeartbeatMonitor) Run(ctx context.Context) error {
	if strings.TrimSpace(m.opts.WorkspaceURL) == "" || strings.TrimSpace(m.opts.ProjectName) == "" || strings.TrimSpace(m.opts.SessionID) == "" {
		return nil
	}

	pidFile := strings.TrimSpace(os.Getenv(heartbeatPIDFileEnv))
	if pidFile != "" {
		if err := writeHeartbeatPIDFile(pidFile, os.Getpid()); err != nil {
			return err
		}
		defer removeHeartbeatPIDFile(pidFile, os.Getpid())
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := m.bootstrapHeartbeat(ctx); err != nil {
		fmt.Printf("⚠️  No se pudo refrescar session file local: %v\n", err)
	}

	ticker := time.NewTicker(m.opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := m.heartbeatIfProjectExists(ctx); err != nil {
				fmt.Printf("⚠️  Heartbeat topology falló: %v\n", err)
			}
		}
	}
}

func (m *topologyHeartbeatMonitor) bootstrapHeartbeat(ctx context.Context) error {
	var lastErr error
	for attempt := 1; attempt <= 6; attempt++ {
		lastErr = m.heartbeatIfProjectExists(ctx)
		if lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
	return lastErr
}

func (m *topologyHeartbeatMonitor) heartbeatIfProjectExists(context.Context) error {
	if _, err := os.Stat(m.opts.ProjectPath); err != nil {
		return nil
	}
	return m.writeSessionFile()
}

func (m *topologyHeartbeatMonitor) sendStatus(ctx context.Context, status string) error {
	token, err := m.tokenFn(ctx, m.cfg)
	if err != nil {
		return err
	}
	payload := map[string]string{
		"session_id":   strings.TrimSpace(m.opts.SessionID),
		"project_name": strings.TrimSpace(m.opts.ProjectName),
		"client_name":  strings.TrimSpace(m.opts.ClientName),
		"source":       strings.TrimSpace(m.opts.Source),
		"status":       strings.TrimSpace(status),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(strings.TrimSpace(m.opts.WorkspaceURL), "/") + "/api/topology/sync-sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func triggerTopologyHeartbeatOnce(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	projectPath, err := os.Getwd()
	if err != nil {
		return err
	}
	monitor := newTopologyHeartbeatMonitor(*cfg, topologyHeartbeatOptions{
		ProjectPath:  projectPath,
		WorkspaceURL: cfg.LastVMHTTPSURL,
		ProjectName:  cfg.LastProjectSlug,
		SessionID:    topologySessionIDForConfig(projectPath, *cfg),
		ClientName:   topologyClientName(*cfg),
		Source:       "mutagen",
		Interval:     topologyHeartbeatInterval,
	})
	return monitor.heartbeatIfProjectExists(context.Background())
}

func startTopologyHeartbeatProcess(cfg *config.Config) error {
	if cfg == nil || strings.TrimSpace(cfg.LastVMHTTPSURL) == "" || strings.TrimSpace(cfg.LastProjectSlug) == "" {
		return nil
	}
	projectPath, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(projectPath); err != nil {
		return err
	}
	pidFile := filepath.Join(projectPath, ".einar", heartbeatPIDFileName)
	alive, err := heartbeatProcessAlive(pidFile)
	if err != nil {
		return err
	}
	if alive {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	runnerPath, err := prepareHeartbeatRunner(exe)
	if err != nil {
		return err
	}
	cmd := exec.Command(runnerPath, "topology-heartbeat")
	cmd.Dir = projectPath
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Env = append(os.Environ(), heartbeatPIDFileEnv+"="+pidFile)
	return cmd.Start()
}

func heartbeatProcessAlive(pidFile string) (bool, error) {
	pidFile = strings.TrimSpace(pidFile)
	if pidFile == "" {
		return false, nil
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		_ = os.Remove(pidFile)
		return false, nil
	}
	alive, err := processExists(pid)
	if err != nil {
		return false, err
	}
	if !alive {
		_ = os.Remove(pidFile)
	}
	return alive, nil
}

func writeHeartbeatPIDFile(pidFile string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0o600)
}

func removeHeartbeatPIDFile(pidFile string, pid int) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	storedPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || storedPID != pid {
		return
	}
	_ = os.Remove(pidFile)
}

func processExists(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").CombinedOutput()
		if err != nil {
			return false, err
		}
		text := strings.TrimSpace(string(out))
		return text != "" && !strings.Contains(strings.ToLower(text), "no tasks are running"), nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false, nil
	}
	return true, nil
}

func prepareHeartbeatRunner(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("heartbeat runner source is empty")
	}
	if runtime.GOOS != "windows" {
		return source, nil
	}
	binDir := filepath.Join(os.TempDir(), "sync-heartbeats")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	target := filepath.Join(binDir, "sync-heartbeat-"+hex.EncodeToString(buf)+".exe")
	if err := copyFile(source, target); err != nil {
		return "", err
	}
	return target, nil
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func topologySessionIDForConfig(projectPath string, cfg config.Config) string {
	if sessionID := strings.TrimSpace(cfg.MutagenSessionName); sessionID != "" {
		return sessionID
	}
	return topologySessionID(projectPath, cfg.LastProjectSlug)
}

func topologySessionID(projectPath, projectName string) string {
	host := localClientName()
	raw := strings.ToLower(strings.TrimSpace(projectName)) + "|" + strings.ToLower(strings.TrimSpace(host)) + "|" + filepath.Clean(strings.TrimSpace(projectPath))
	sum := sha1.Sum([]byte(raw))
	return "sync-" + hex.EncodeToString(sum[:8])
}

func topologyClientName(cfg config.Config) string {
	if email := extractEmailFromJWT(cfg.Token); email != "" {
		return email
	}
	if email := strings.ToLower(strings.TrimSpace(cfg.UserEmail)); strings.Contains(email, "@") {
		return email
	}
	return localClientName()
}

func (m *topologyHeartbeatMonitor) writeSessionFile() error {
	projectPath := strings.TrimSpace(m.opts.ProjectPath)
	sessionID := strings.TrimSpace(m.opts.SessionID)
	projectName := strings.TrimSpace(m.opts.ProjectName)
	if projectPath == "" || sessionID == "" || projectName == "" {
		return nil
	}
	hostname, _ := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	email := extractEmailFromJWT(m.cfg.Token)
	if email == "" {
		email = strings.ToLower(strings.TrimSpace(m.cfg.UserEmail))
	}
	if email == "" {
		candidate := strings.ToLower(strings.TrimSpace(topologyClientName(m.cfg)))
		if strings.Contains(candidate, "@") {
			email = candidate
		}
	}
	payload := topologySessionFile{
		SessionID:   sessionID,
		ProjectName: projectName,
		Email:       email,
		Hostname:    hostname,
		LastSeenAt:  m.now().UTC(),
		Source:      "cli-file",
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(projectPath, ".einar", "sessions", topologySessionFileName(sessionID, hostname)+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func topologySessionFileName(sessionID, hostname string) string {
	sessionID = sanitizeSessionFilePart(sessionID)
	hostname = sanitizeSessionFilePart(hostname)
	switch {
	case sessionID == "":
		return hostname
	case hostname == "":
		return sessionID
	default:
		return sessionID + "--" + hostname
	}
}

func sanitizeSessionFilePart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer("<", "-", ">", "-", ":", "-", "\"", "-", "/", "-", "\\", "-", "|", "-", "?", "-", "*", "-", " ", "-")
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-.")
}

func mergeTopologyIdentityConfig(cfg config.Config) config.Config {
	if globalCfg, err := config.LoadGlobal(); err == nil {
		cfg = overlayTopologyIdentityConfig(cfg, globalCfg)
	}
	if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.UserEmail) == "" {
		if ancestorCfg, err := loadAncestorTopologyIdentityConfig(); err == nil {
			cfg = overlayTopologyIdentityConfig(cfg, ancestorCfg)
		}
	}
	return cfg
}

func overlayTopologyIdentityConfig(base, extra config.Config) config.Config {
	if token := strings.TrimSpace(extra.Token); token != "" {
		base.Token = token
	}
	if refresh := strings.TrimSpace(extra.RefreshToken); refresh != "" {
		base.RefreshToken = refresh
	}
	if email := strings.TrimSpace(extra.UserEmail); email != "" {
		base.UserEmail = email
	}
	if apiURL := strings.TrimSpace(extra.APIURL); apiURL != "" {
		base.APIURL = apiURL
	}
	return base
}

func loadAncestorTopologyIdentityConfig() (config.Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return config.Config{}, err
	}
	current := filepath.Clean(wd)
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return config.Config{}, fmt.Errorf("ancestor global config not found")
		}
		candidate := filepath.Join(parent, ".einar", "config.json")
		b, err := os.ReadFile(candidate)
		if err == nil {
			var cfg config.Config
			if err := json.Unmarshal(b, &cfg); err == nil {
				return cfg, nil
			}
		}
		current = parent
	}
}

func localClientName() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "local-client"
	}
	return strings.TrimSpace(host)
}

func init() {
	rootCmd.AddCommand(topologyHeartbeatCmd)
}
