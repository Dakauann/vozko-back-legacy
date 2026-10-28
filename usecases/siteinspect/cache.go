package siteinspect

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"time"

	"vozko/domain/site"
)

const (
	InspectionKeep      = 2 * time.Hour
	inspectionKeyPrefix = "site:inspect:advisory:"
)

type InspectionCache interface {
	GetString(key string) (string, error)
	SetString(key string, value string, ttl time.Duration) error
}

func inspectionKey(build string, routes []string, settings site.Settings) (string, bool) {
	if build == "" {
		return "", false
	}
	raw, err := json.Marshal(struct {
		Routes   []string      `json:"routes"`
		Settings site.Settings `json:"settings"`
	}{routes, settings})
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(raw)
	return inspectionKeyPrefix + build + ":" + hex.EncodeToString(sum[:]), true
}

func (s Service) cachedInspection(key string) (Report, bool) {
	if s.Cache == nil {
		return Report{}, false
	}
	raw, err := s.Cache.GetString(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return Report{}, false
	}
	var report Report
	if json.Unmarshal([]byte(raw), &report) != nil || !report.Known {
		return Report{}, false
	}
	return report, true
}

func (s Service) keepInspection(key string, report Report) {
	if s.Cache == nil || !report.Known {
		return
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return
	}
	if err := s.Cache.SetString(key, string(raw), InspectionKeep); err != nil {
		log.Printf("[sites] an inspection was not kept: %v", err)
	}
}
