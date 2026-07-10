package mounted

import (
	"net/http"
	"strings"
)

const ReturnToCookieName = "app_return_to"

func Prefix(r *http.Request) string {
	if r == nil {
		return ""
	}
	prefix := strings.TrimSpace(r.Header.Get("X-Forwarded-Prefix"))
	if prefix == "" {
		prefix = previewPrefixFromPath(r.URL.Path)
	}
	if prefix == "" {
		return ""
	}
	prefix = NormalizePath(prefix)
	return strings.TrimRight(prefix, "/") + "/"
}

func previewPrefixFromPath(path string) string {
	path = NormalizePath(path)
	const marker = "/preview"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	prefix := path[:idx+len(marker)]
	if !strings.HasPrefix(prefix, "/agent/sessions/") {
		return ""
	}
	return prefix
}

func App(prefix, path string) string {
	path = NormalizePath(path)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return path
	}
	prefix = strings.TrimRight(NormalizePath(prefix), "/")
	if path == "/" {
		return prefix + "/"
	}
	return prefix + path
}

func Host(path string) string {
	return NormalizePath(path)
}

func Relative(prefix, path string) string {
	path = NormalizePath(path)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return path
	}
	trimmedPrefix := strings.TrimRight(NormalizePath(prefix), "/")
	if path == trimmedPrefix {
		return "/"
	}
	if strings.HasPrefix(path, trimmedPrefix+"/") {
		return NormalizePath(strings.TrimPrefix(path, trimmedPrefix))
	}
	return path
}

func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func CurrentAppURL(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "/"
	}
	prefix := Prefix(r)
	path := App(prefix, Relative(prefix, r.URL.Path))
	if raw := strings.TrimSpace(r.URL.RawQuery); raw != "" {
		return path + "?" + raw
	}
	return path
}

func SetReturnToCookie(w http.ResponseWriter, r *http.Request, value string, secure bool) {
	if w == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = CurrentAppURL(r)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     ReturnToCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ReadReturnTo(r *http.Request) string {
	if r == nil {
		return ""
	}
	cookie, err := r.Cookie(ReturnToCookieName)
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(cookie.Value)
	if !IsSafeReturnTo(value) {
		return ""
	}
	return value
}

func IsSafeReturnTo(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if !strings.HasPrefix(value, "/") {
		return false
	}
	return !strings.HasPrefix(value, "//")
}

func ClearReturnToCookie(w http.ResponseWriter, secure bool) {
	if w == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     ReturnToCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
