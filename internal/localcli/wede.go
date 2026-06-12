package localcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/config"
)

const defaultWedeVersion = "v1.0.2"

func ensureWedeInstalledOnProvisionedVM(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config nil")
	}

	target := strings.TrimSpace(cfg.LastVMSshDest)
	remotePath := ""
	if target == "" {
		if t, p, ok := sshTargetAndPathFromMutagenDestination(strings.TrimSpace(cfg.MutagenDestination)); ok {
			target = t
			remotePath = p
		}
	} else if _, p, ok := sshTargetAndPathFromMutagenDestination(strings.TrimSpace(cfg.MutagenDestination)); ok {
		remotePath = p
	}
	if target == "" {
		return nil
	}
	if strings.TrimSpace(remotePath) == "" {
		remotePath = "$HOME"
	}

	script := fmt.Sprintf(`set -euo pipefail

VERSION=%q
INSTALL_DIR="$HOME/.local/bin"
BIN="$INSTALL_DIR/wede"
BASHRC="$HOME/.bashrc"
PIDFILE="$HOME/.wede.pid"
LOGFILE="$HOME/.wede.log"
PROJECT_DIR=%q
CONFIG_DIR="$HOME/.config/wede"
CONFIG_FILE="$CONFIG_DIR/wede.config.json"
PROJECT_CONFIG_FILE="$PROJECT_DIR/wede.config.json"
WEDE_PASSWORD="wede-dev"

mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$PROJECT_DIR"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)
    ASSET="wede-linux-amd64"
    CHECKSUM="4633c8f0e50fa2541135b8cd235c3613ad61ff9387da4fd526805aec253b2eec"
    ;;
  aarch64|arm64)
    ASSET="wede-linux-arm64"
    CHECKSUM="d0276ca3d08ee9820ce4c4b5e895ad6e998d022bef557d59ba59f184eca781d3"
    ;;
  *)
    echo "Arquitectura Linux no soportada para wede: $ARCH" >&2
    exit 1
    ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
  SHACMD='sha256sum'
elif command -v shasum >/dev/null 2>&1; then
  SHACMD='shasum -a 256'
else
  echo "No hay sha256sum ni shasum disponibles en la VM" >&2
  exit 1
fi

need_install=1
if [ -x "$BIN" ]; then
  current_sum=$(eval "$SHACMD \"$BIN\"" | awk '{print $1}')
  if [ "$current_sum" = "$CHECKSUM" ]; then
    need_install=0
    echo "wede ya está instalado en $BIN"
  fi
fi

if [ "$need_install" -eq 1 ]; then
  TMP_BIN="$(mktemp)"
  URL="https://github.com/Ignaciojeria/wede/releases/download/${VERSION}/${ASSET}"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$TMP_BIN"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$TMP_BIN" "$URL"
  else
    echo "No hay curl ni wget disponibles en la VM" >&2
    exit 1
  fi

  downloaded_sum=$(eval "$SHACMD \"$TMP_BIN\"" | awk '{print $1}')
  if [ "$downloaded_sum" != "$CHECKSUM" ]; then
    echo "Checksum inválido para $ASSET" >&2
    rm -f "$TMP_BIN"
    exit 1
  fi

  install -m 0755 "$TMP_BIN" "$BIN"
  rm -f "$TMP_BIN"
  echo "wede instalado en $BIN"
fi

PATH_LINE='export PATH="$HOME/.local/bin:$PATH"'
if [ -f "$BASHRC" ]; then
  if ! grep -Fq "$PATH_LINE" "$BASHRC"; then
    printf '\n%%s\n' "$PATH_LINE" >> "$BASHRC"
    echo "PATH actualizado en $BASHRC"
  fi
else
  printf '%%s\n' "$PATH_LINE" > "$BASHRC"
  echo "PATH inicializado en $BASHRC"
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

is_running=0
if [ -f "$PIDFILE" ]; then
  existing_pid=$(cat "$PIDFILE" 2>/dev/null || true)
  if [ -n "$existing_pid" ] && kill -0 "$existing_pid" 2>/dev/null; then
    is_running=1
    echo "wede ya está corriendo con pid $existing_pid"
  else
    rm -f "$PIDFILE"
  fi
fi

if [ "$is_running" -eq 0 ]; then
  cd "$PROJECT_DIR"
  nohup "$BIN" > "$LOGFILE" 2>&1 < /dev/null &
  new_pid=$!
  echo "$new_pid" > "$PIDFILE"
  sleep 2
  if ! kill -0 "$new_pid" 2>/dev/null; then
    echo "wede no logró iniciar; revisa $LOGFILE" >&2
    tail -n 50 "$LOGFILE" 2>/dev/null || true
    exit 1
  fi
  echo "wede iniciado con pid $new_pid usando puerto por defecto"
fi

printf 'WEDE_OK version=%%s path=%%s pidfile=%%s logfile=%%s config=%%s project_config=%%s\n' "$VERSION" "$BIN" "$PIDFILE" "$LOGFILE" "$CONFIG_FILE" "$PROJECT_CONFIG_FILE"
`, defaultWedeVersion, remotePath)

	out, err := runSSHScriptWithTimeout(target, script, 2*time.Minute)
	if err != nil {
		if strings.TrimSpace(out) != "" {
			return fmt.Errorf("%w | salida remota: %s", err, out)
		}
		return err
	}
	if strings.TrimSpace(out) != "" {
		fmt.Println(out)
	}
	return nil
}
