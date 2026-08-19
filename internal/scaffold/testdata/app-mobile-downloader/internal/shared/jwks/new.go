package jwks

import (
	"strings"

	"gitinittest5/internal/shared/configuration"

	"github.com/MicahParks/keyfunc/v3"
)

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
