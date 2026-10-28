package sitepublish

import (
	"net"
	"net/url"
	"regexp"
	"strings"

	"vozko/domain/hostname"
)

const (
	previewLabelPrefix    = "preview--"
	deploymentLabelPrefix = "deploy--"
)

var deploymentAddress = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func PlatformDomains(public string, application []string) []string {
	return append([]string{public}, application...)
}

type Origins struct {
	LoopbackPort string
}

func (o Origins) Of(host string) string {
	name := hostname.Normalize(host)
	if hostname.Loopback(name) {
		return "http://" + net.JoinHostPort(name, o.LoopbackPort)
	}
	return "https://" + name
}

type Addresses struct {
	Domain  string
	Origins Origins
}

func (a Addresses) domain() string {
	return hostname.Normalize(a.Domain)
}

func (a Addresses) PreviewHost(token string) string {
	return previewLabelPrefix + token + "." + a.domain()
}

func (a Addresses) PreviewURL(token string) string {
	return a.Origins.Of(a.PreviewHost(token)) + "/"
}

func (a Addresses) DeploymentURL(deploymentID string) string {
	return a.Origins.Of(deploymentLabelPrefix+deploymentID+"."+a.domain()) + "/"
}

func (a Addresses) labelWith(host, prefix string) (string, bool) {
	label, ok := hostname.SubdomainLabel(host, a.domain())
	if !ok {
		return "", false
	}
	rest, ok := strings.CutPrefix(label, prefix)
	return rest, ok && rest != ""
}

func (a Addresses) PreviewToken(host string) (string, bool) {
	return a.labelWith(host, previewLabelPrefix)
}

func (a Addresses) DeploymentID(host string) (string, bool) {
	id, ok := a.labelWith(host, deploymentLabelPrefix)
	return id, ok && deploymentAddress.MatchString(id)
}

func (a Addresses) Within(host string) bool {
	name, domain := hostname.Normalize(host), a.domain()
	return domain != "" && (name == domain || strings.HasSuffix(name, "."+domain))
}

func (a Addresses) PreviewOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Path != "" || u.RawQuery != "" || u.User != nil {
		return false
	}
	_, ok := a.PreviewToken(u.Hostname())
	return ok && a.Origins.Of(u.Hostname()) == strings.ToLower(u.Scheme+"://"+u.Host)
}
