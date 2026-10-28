package sitepublish

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"vozko/domain/site"
	"vozko/domain/strictjson"
)

const (
	ServingVersion       = 1
	ControlVariant       = "control"
	hostKeyPrefix        = "host:"
	minHashIterations    = 100_000
	totalVariantWeight   = 100
	passwordHashSections = 4
)

var ErrServingInvalid = errors.New("sitepublish: the serving settings are invalid")

var (
	deploymentID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	variantKey   = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
	hashPart     = regexp.MustCompile(`^[A-Za-z0-9+/_=-]+$`)
)

type Variant struct {
	Key    string `json:"key"`
	Weight int    `json:"weight"`
}

type Experiment struct {
	Route    string    `json:"route"`
	Variants []Variant `json:"variants"`
}

type ServingSettings struct {
	Version      int             `json:"v"`
	DeploymentID string          `json:"deployment"`
	Suspended    bool            `json:"suspended,omitempty"`
	PasswordHash string          `json:"password_hash,omitempty"`
	Experiments  []Experiment    `json:"experiments,omitempty"`
	Redirects    []site.Redirect `json:"redirects,omitempty"`
}

func HostKey(hostname string) string {
	return hostKeyPrefix + hostname
}

func servingError(reason string) error {
	return fmt.Errorf("%w: %s", ErrServingInvalid, reason)
}

func (s ServingSettings) Validate() error {
	if !deploymentID.MatchString(s.DeploymentID) {
		return servingError("the deployment id is malformed")
	}
	if s.PasswordHash != "" && !validPasswordHash(s.PasswordHash) {
		return servingError("the password must be a pbkdf2-sha256 hash")
	}
	routes := map[string]bool{}
	for _, e := range s.Experiments {
		if !site.ValidRoutePath(e.Route) || routes[e.Route] {
			return servingError("experiment route " + e.Route + " is invalid or repeated")
		}
		routes[e.Route] = true
		if err := e.validate(); err != nil {
			return err
		}
	}
	if err := site.ValidateRedirects(s.Redirects); err != nil {
		return fmt.Errorf("%w: %w", ErrServingInvalid, err)
	}
	return nil
}

func (e Experiment) validate() error {
	if len(e.Variants) < 2 {
		return servingError("an experiment needs at least two variants")
	}
	keys := map[string]bool{}
	total := 0
	for _, v := range e.Variants {
		if !variantKey.MatchString(v.Key) || keys[v.Key] || v.Weight < 0 {
			return servingError("variant " + v.Key + " is invalid")
		}
		keys[v.Key] = true
		total += v.Weight
	}
	if !keys[ControlVariant] || total != totalVariantWeight {
		return servingError("variants need a control and weights adding up to 100")
	}
	return nil
}

func validPasswordHash(hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != passwordHashSections || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	return err == nil && iterations >= minHashIterations && len(parts[2]) >= 16 && len(parts[3]) >= 32 && hashPart.MatchString(parts[2]) && hashPart.MatchString(parts[3])
}

func (s ServingSettings) Encode() ([]byte, error) {
	s.Version = ServingVersion
	if err := s.Validate(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(out.Bytes()), nil
}

func DecodeServing(raw []byte) (ServingSettings, error) {
	var s ServingSettings
	if err := strictjson.Decode(raw, &s); err != nil {
		return ServingSettings{}, fmt.Errorf("%w: %w", ErrServingInvalid, err)
	}
	if s.Version != ServingVersion {
		return ServingSettings{}, servingError("unknown version")
	}
	if err := s.Validate(); err != nil {
		return ServingSettings{}, err
	}
	return s, nil
}

func ValidVariantKey(key string) bool {
	return variantKey.MatchString(key)
}
