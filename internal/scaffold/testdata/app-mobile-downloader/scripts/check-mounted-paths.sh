#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

pattern='href="/|action="/|hx-(get|post|put|delete|patch)="/'

# ponytail: keep the guardrail dumb and loud; if host-aware exceptions ever need to
# live under internal/, add a tiny allowlist here instead of weakening the rule.
if rg -n --pcre2 "$pattern" internal --glob '*.templ' --glob '!**/*_templ.go'; then
  echo
  echo "mounted-path guard failed: usa nav.AppPath()/nav.HostPath() o helpers prefix-aware en templates de internal/."
  exit 1
fi

echo "mounted-path guard OK"
