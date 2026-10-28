package sitepublish

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"vozko/domain/emailaddress"
	"vozko/domain/shared"
)

const (
	MinSlugLength = 3
	MaxSlugLength = 63
)

var (
	ErrHostnameInvalid   = errors.New("sitepublish: the hostname is not a valid domain name")
	ErrHostnameForbidden = errors.New("sitepublish: the hostname belongs to a reserved or platform domain")
	ErrSlugInvalid       = errors.New("sitepublish: the subdomain is not a lowercase dns label")
	ErrSlugReserved      = errors.New("sitepublish: the subdomain is reserved")
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var reservedSlugs = map[string]bool{
	"www": true, "api": true, "app": true, "admin": true, "mail": true, "email": true, "smtp": true, "imap": true,
	"pop": true, "ftp": true, "cdn": true, "static": true, "assets": true, "media": true, "preview": true,
	"previews": true, "site": true, "sites": true, "docs": true, "help": true, "support": true, "status": true,
	"blog": true, "login": true, "auth": true, "oauth": true, "sso": true, "account": true, "accounts": true,
	"billing": true, "pay": true, "payment": true, "payments": true, "checkout": true, "secure": true,
	"security": true, "dashboard": true, "panel": true, "abuse": true, "postmaster": true, "webmaster": true,
	"hostmaster": true, "root": true, "ns": true, "ns1": true, "ns2": true, "dns": true, "mx": true, "test": true,
	"dev": true, "staging": true, "homolog": true, "beta": true, "internal": true, "git": true, "wpad": true,
	"autodiscover": true, "autoconfig": true, "report": true, "reports": true,
}

var reservedTLDs = map[string]bool{"local": true, "localhost": true, "test": true, "example": true, "invalid": true, "internal": true, "home": true, "lan": true, "onion": true}

func ValidateCustomHostname(raw string, platformDomains []string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.ContainsAny(trimmed, "/:@") {
		return "", ErrHostnameInvalid
	}
	host, err := emailaddress.NormalizeDomain(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrHostnameInvalid, raw)
	}
	labels := strings.Split(host, ".")
	if reservedTLDs[labels[len(labels)-1]] {
		return "", fmt.Errorf("%w: %s", ErrHostnameForbidden, host)
	}
	for _, domain := range platformDomains {
		owned, err := emailaddress.NormalizeDomain(domain)
		if err != nil {
			return "", fmt.Errorf("%w: platform domain %s is malformed", ErrHostnameForbidden, domain)
		}
		if host == owned || strings.HasSuffix(host, "."+owned) {
			return "", fmt.Errorf("%w: %s", ErrHostnameForbidden, host)
		}
	}
	return host, nil
}

func SlugCandidate(text string) string {
	var b strings.Builder
	pending := false
	for _, r := range shared.FoldForMatch(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pending && b.Len() > 0 {
				b.WriteByte('-')
			}
			pending = false
			b.WriteRune(r)
			continue
		}
		pending = true
	}
	slug := b.String()
	if len(slug) > MaxSlugLength {
		slug = strings.TrimRight(slug[:MaxSlugLength], "-")
	}
	return slug
}

func PlatformSlug(page, workspace string) string {
	return SlugCandidate(page + " " + workspace)
}

func ValidateSlug(slug string, reserved ...string) error {
	if len(slug) < MinSlugLength || len(slug) > MaxSlugLength || !slugPattern.MatchString(slug) {
		return fmt.Errorf("%w: %q", ErrSlugInvalid, slug)
	}
	if reservedSlugs[slug] {
		return fmt.Errorf("%w: %q", ErrSlugReserved, slug)
	}
	for _, word := range reserved {
		if SlugCandidate(word) == slug {
			return fmt.Errorf("%w: %q", ErrSlugReserved, slug)
		}
	}
	return nil
}

func PlatformHostname(slug, publicDomain string, reserved ...string) (string, error) {
	if err := ValidateSlug(slug, reserved...); err != nil {
		return "", err
	}
	domain, err := emailaddress.NormalizeDomain(publicDomain)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrHostnameInvalid, publicDomain)
	}
	host, err := emailaddress.NormalizeDomain(slug + "." + domain)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrHostnameInvalid, slug)
	}
	return host, nil
}
