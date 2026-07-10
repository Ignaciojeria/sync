package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	templateDirName   = "app-mobile-downloader"
	sourceModuleName  = "scaffoldxd1"
	sourceProjectName = "scaffoldxd1"
)

//go:embed all:testdata/app-mobile-downloader
var templatesFS embed.FS

func MaterializeAppMobileDownloader(destination, moduleName, bootstrapEmail string) error {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		destination = "."
	}
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		moduleName = sourceModuleName
	}
	bootstrapEmail = strings.ToLower(strings.TrimSpace(bootstrapEmail))

	root := filepath.ToSlash(filepath.Join("testdata", templateDirName))
	return fs.WalkDir(templatesFS, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "go.mod.tmpl" {
			rel = "go.mod"
		}
		rel = filepath.FromSlash(rel)
		outPath := filepath.Join(destination, rel)

		if entry.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}
		if _, err := os.Stat(outPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}

		data, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		data = renderModuleName(data, moduleName)
		if filepath.ToSlash(rel) == "internal/shared/access.go" {
			data = renderBootstrapEmail(data, bootstrapEmail)
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		mode := fileMode(rel, entry)
		if err := os.WriteFile(outPath, data, mode); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		return nil
	})
}

func renderModuleName(data []byte, moduleName string) []byte {
	if moduleName == "" {
		moduleName = sourceModuleName
	}
	if moduleName == sourceModuleName {
		return data
	}
	content := string(data)
	content = strings.ReplaceAll(content, "module "+sourceModuleName, "module "+moduleName)
	content = strings.ReplaceAll(content, sourceModuleName+"/", moduleName+"/")
	content = strings.ReplaceAll(content, sourceModuleName, moduleName)
	if sourceProjectName != sourceModuleName {
		content = strings.ReplaceAll(content, sourceProjectName, moduleName)
	}
	return []byte(content)
}

func renderBootstrapEmail(data []byte, email string) []byte {
	if email == "" || !strings.Contains(email, "@") {
		return data
	}
	content := string(data)
	entry := fmt.Sprintf("\t%q: {},", email)
	if strings.Contains(content, entry) {
		return data
	}
	content = insertMapEntry(content, "var allowedAppEmails = map[string]struct{}{", entry)
	content = insertMapEntry(content, "var allowedEditorEmails = map[string]struct{}{", entry)
	return []byte(content)
}

func insertMapEntry(content, marker, entry string) string {
	idx := strings.Index(content, marker)
	if idx < 0 {
		return content
	}
	insertAt := idx + len(marker)
	return content[:insertAt] + "\n" + entry + content[insertAt:]
}

func fileMode(rel string, entry fs.DirEntry) fs.FileMode {
	info, err := entry.Info()
	if err != nil {
		return 0o644
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	mode |= 0o600
	unixRel := filepath.ToSlash(rel)
	if strings.HasPrefix(unixRel, ".githooks/") || (strings.HasPrefix(unixRel, "scripts/") && strings.HasSuffix(unixRel, ".sh")) {
		mode |= 0o111
	}
	return mode
}
