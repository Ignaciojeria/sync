package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ignaciojeria/sync/internal/api"
	"github.com/Ignaciojeria/sync/internal/config"
)

// TestAIGatewayEndToEnd simula la respuesta de POST /api/projects con
// aiGateway inline y verifica que materializeProjectEnv escribe las env vars
// SYNC_AI_GATEWAY_BASE_URL y SYNC_AI_GATEWAY_API_KEY en .env.
func TestAIGatewayEndToEnd(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	resp := &api.CreateProjectResponse{
		ProjectID: "prj_test01",
		Slug:      "acme-copilot",
		Workspace: struct {
			Branch string `json:"branch"`
			Mode   string `json:"mode"`
		}{Branch: "main", Mode: "single-owner"},
	}
	resp.VM.Name = "acme-copilot"
	resp.VM.HTTPSURL = "https://acme-copilot.einar.exe.xyz"
	resp.VM.SSHDestination = "einar@acme-copilot.einar.exe.xyz"
	resp.Sync.Destination = "einar@acme-copilot.einar.exe.xyz"
	resp.Sync.SessionName = "acme-copilot"
	resp.Sync.IgnoreVCS = true
	resp.Database.Name = "acme_copilot"
	resp.Database.User = "acme_copilot"
	resp.Database.Host = "db"
	resp.Database.Port = 5432
	resp.AIGateway = &api.ProjectAIGateway{
		Provider:   "aigateway",
		APIBaseURL: "https://sync-ai-gateway.exe.xyz",
		ClientID:   "cl_acme-copilot-7K3mXq",
		ClientName: "Acme · Copilot",
		ClientEmail: "ops@acme.com",
		KeyLabel:   "project-acme-copilot-prod",
		KeyID:      "b8b6b6d7-0a6a-4b9d-8a92-8f12c3d4e5f6",
		KeyPrefix:  "sag_4e54550f",
		APIKey:     "sag_4e54550ff2f396c2c0debd7830e9e478a3d02513bebb094b",
		APIKeyRef:  "/srv/projects/acme-copilot/secrets/aigw-api-key",
	}

	gw := normalizeProjectAIGateway(resp)
	if gw == nil {
		t.Fatal("normalizeProjectAIGateway devolvió nil con respuesta válida")
	}

	cfg := config.Config{
		LastProjectSlug:    "acme-copilot",
		ProjectDBName:      "acme_copilot",
		ProjectDBUser:      "acme_copilot",
		ProjectDBHost:      "db",
		ProjectDBPort:      5432,
		ProjectDatabaseURL: "postgres://acme_copilot:pw@db:5432/acme_copilot",
		// Canonical: el backend aprovisiona aiGateway y la cfg se hidrata
		// desde la respuesta (sin fallbacks hardcoded en materializeProjectEnv).
		AIGatewayAPIBaseURL: gw.APIBaseURL,
		AIGatewayAPIKey:     gw.APIKey,
		AIGatewayAPIKeyRef:  gw.APIKeyRef,
	}

	if err := materializeProjectEnv(cfg); err != nil {
		t.Fatalf("materializeProjectEnv: %v", err)
	}

	envBytes, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	env := string(envBytes)

	wants := map[string]string{
		"SYNC_AI_GATEWAY_BASE_URL": "https://sync-ai-gateway.exe.xyz",
		"SYNC_AI_GATEWAY_API_KEY":  "sag_4e54550ff2f396c2c0debd7830e9e478a3d02513bebb094b",
	}
	for k, want := range wants {
		if !strings.Contains(env, k+"="+want) {
			t.Errorf(".env no contiene %s=%s\n---\n%s\n---", k, want, env)
		}
	}

	secretsPath := filepath.Join(os.TempDir(), "aigw-smoke-test")
	t.Cleanup(func() { _ = os.RemoveAll(secretsPath) })
	// os.UserHomeDir en Windows lee USERPROFILE; en *nix lee HOME.
	if err := os.Setenv("HOME", secretsPath); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Setenv("USERPROFILE", secretsPath); err != nil {
		t.Fatalf("set USERPROFILE: %v", err)
	}
	if err := writeProjectSecretsBundle("acme-copilot", "", "", "", "", "", "", ""); err != nil {
		t.Fatalf("writeProjectSecretsBundle: %v", err)
	}
	// aigw-api-key no debe existir cuando el backend no aprovisionó key.
	if _, err := os.Stat(filepath.Join(secretsPath, ".einar", "projects", "acme-copilot", "secrets", "aigw-api-key")); !os.IsNotExist(err) {
		t.Errorf("aigw-api-key no debió existir sin apiKey del backend: %v", err)
	}
}