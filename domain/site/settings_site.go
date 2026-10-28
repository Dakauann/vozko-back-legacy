package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

type Identity struct {
	FaviconMediaID string `json:"favicon_media_id,omitempty"`
	StructuredData bool   `json:"structured_data,omitempty"`
}

func (i Identity) validate() error {
	if i.FaviconMediaID != "" && !referenceID.MatchString(i.FaviconMediaID) {
		return settingsError("the favicon is not a media id")
	}
	return nil
}

var ErrOrganizationInvalid = errors.New("site: the organization data is invalid")

const maxOrganizationNameRunes = 120

type organizationData struct {
	Context string `json:"@context"`
	Type    string `json:"@type"`
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Logo    string `json:"logo,omitempty"`
}

func OrganizationData(name, origin, logo string) (json.RawMessage, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxOrganizationNameRunes {
		return nil, fmt.Errorf("%w: the name needs 1 to %d characters", ErrOrganizationInvalid, maxOrganizationNameRunes)
	}
	if origin != "" && !httpsOrigin(origin) {
		return nil, fmt.Errorf("%w: the url must be an https origin", ErrOrganizationInvalid)
	}
	if logo != "" && !httpsURL(logo) {
		return nil, fmt.Errorf("%w: the logo must be an https address", ErrOrganizationInvalid)
	}
	return json.Marshal(organizationData{Context: "https://schema.org", Type: "Organization", Name: name, URL: origin, Logo: logo})
}

func httpsOrigin(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && httpsURL(raw) && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}
