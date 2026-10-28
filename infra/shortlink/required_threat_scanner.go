package shortlink

import (
	"errors"
	"net/http"
	"strings"

	"vozko/domain/shortlink"
)

var ErrScannerKeyMissing = errors.New("shortlink: a required safe browsing scanner has no api key")

func NewRequiredThreatScanner(apiKey, endpoint string, client *http.Client) (shortlink.ThreatScanner, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, ErrScannerKeyMissing
	}
	return NewThreatScanner(key, endpoint, client), nil
}
