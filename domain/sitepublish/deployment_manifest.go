package sitepublish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"vozko/domain/site"
	"vozko/domain/strictjson"
)

const (
	ManifestVersion = 1
	MediaPathPrefix = "/_m/"
	maxPathLength   = 512
)

var ErrManifestInvalid = errors.New("sitepublish: the deployment manifest is invalid")

var (
	shaHex        = regexp.MustCompile(`^[a-f0-9]{64}$`)
	servedPath    = regexp.MustCompile(`^/[A-Za-z0-9._~@/-]*$`)
	hashedName    = regexp.MustCompile(`[.-][A-Za-z0-9_]{8,}\.[a-z0-9]+$`)
	documentTypes = map[string]bool{".html": true, ".txt": true, ".xml": true, ".json": true, ".webmanifest": true}
)

type FileEntry struct {
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Immutable   bool   `json:"immutable,omitempty"`
	CountView   bool   `json:"view,omitempty"`
}

type DeploymentManifest struct {
	Version      int                          `json:"v"`
	DeploymentID string                       `json:"deployment"`
	ProjectID    string                       `json:"site"`
	APIOrigin    string                       `json:"api"`
	Files        map[string]FileEntry         `json:"files"`
	Routes       map[string]string            `json:"routes"`
	NotFound     string                       `json:"not_found,omitempty"`
	Variants     map[string]map[string]string `json:"variants,omitempty"`
	Integrations []site.IntegrationKind       `json:"integrations,omitempty"`
	Policy       site.ContentPolicy           `json:"policy"`
}

func SHA256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func NewFileEntry(body []byte, contentType, servedAt string) FileEntry {
	return FileEntry{SHA256: SHA256Hex(body), ContentType: contentType, Size: int64(len(body)), Immutable: ImmutablePath(servedAt)}
}

func ImmutablePath(p string) bool {
	if strings.HasPrefix(p, MediaPathPrefix) {
		return true
	}
	if documentTypes[strings.ToLower(path.Ext(p))] {
		return false
	}
	return hashedName.MatchString(path.Base(p))
}

func BlobKey(sha string) string {
	return "blobs/" + sha
}

func ManifestKey(deploymentID string) string {
	return "deploys/" + deploymentID + ".json"
}

func manifestError(reason string) error {
	return fmt.Errorf("%w: %s", ErrManifestInvalid, reason)
}

func ValidServedPath(p string) bool {
	if len(p) > maxPathLength || !servedPath.MatchString(p) || strings.HasSuffix(p, "/") {
		return false
	}
	return path.Clean(p) == p
}

func (m DeploymentManifest) Validate() error {
	if !deploymentID.MatchString(m.DeploymentID) {
		return manifestError("the deployment id is malformed")
	}
	if strings.TrimSpace(m.ProjectID) == "" || !deploymentID.MatchString(m.ProjectID) {
		return manifestError("the site id is malformed")
	}
	if u, err := url.Parse(m.APIOrigin); err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.User != nil {
		return manifestError("the api origin must be an https origin")
	}
	if len(m.Files) == 0 {
		return manifestError("a deployment needs files")
	}
	for p, entry := range m.Files {
		if !ValidServedPath(p) {
			return manifestError("path " + p + " is not servable")
		}
		if !shaHex.MatchString(entry.SHA256) || strings.TrimSpace(entry.ContentType) == "" || entry.Size < 0 {
			return manifestError("file " + p + " is incomplete")
		}
	}
	if _, ok := m.Routes[site.HomeRoute]; !ok {
		return manifestError("the home route is missing")
	}
	for route, file := range m.Routes {
		if !site.ValidRoutePath(route) {
			return manifestError("route " + route + " is not a route path")
		}
		if _, ok := m.Files[file]; !ok {
			return manifestError("route " + route + " points to a missing file")
		}
	}
	for route, variants := range m.Variants {
		if _, ok := m.Routes[route]; !ok {
			return manifestError("variants of " + route + " belong to no route")
		}
		for key, file := range variants {
			if !variantKey.MatchString(key) || key == ControlVariant {
				return manifestError("variant " + key + " of " + route + " is invalid")
			}
			if _, ok := m.Files[file]; !ok {
				return manifestError("variant " + key + " of " + route + " points to a missing file")
			}
		}
	}
	if m.NotFound != "" {
		if _, ok := m.Files[m.NotFound]; !ok {
			return manifestError("the not found page is missing")
		}
	}
	for _, kind := range m.Integrations {
		if !slices.Contains(site.IntegrationKinds(), kind) {
			return manifestError("integration " + string(kind) + " is unknown")
		}
	}
	if err := m.Policy.Validate(); err != nil {
		return manifestError(err.Error())
	}
	return nil
}

func (m DeploymentManifest) Encode() ([]byte, error) {
	m.Version = ManifestVersion
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(out.Bytes()), nil
}

func DecodeDeploymentManifest(raw []byte) (DeploymentManifest, error) {
	var m DeploymentManifest
	if err := strictjson.Decode(raw, &m); err != nil {
		return DeploymentManifest{}, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	if m.Version != ManifestVersion {
		return DeploymentManifest{}, manifestError("unknown version")
	}
	if err := m.Validate(); err != nil {
		return DeploymentManifest{}, err
	}
	return m, nil
}
