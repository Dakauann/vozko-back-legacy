package netguard

import (
	"context"
	"errors"
	"net"
	"strings"

	"vozko/domain/agenttool"
)

type HostVerdicts struct {
	resolver IPResolver
}

func NewHostVerdicts(resolver IPResolver) *HostVerdicts {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &HostVerdicts{resolver: resolver}
}

func (h *HostVerdicts) ClassifyHost(ctx context.Context, host string) (agenttool.HostVerdict, error) {
	host = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(host), "["), "]")
	if ip := net.ParseIP(host); ip != nil {
		if IsBlockedIP(ip) {
			return agenttool.HostBlocked, nil
		}
		return agenttool.HostOK, nil
	}
	addrs, err := h.resolver.LookupIPAddr(ctx, host)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return agenttool.HostNotFound, nil
	}
	if err != nil {
		return agenttool.HostOK, err
	}
	if len(addrs) == 0 {
		return agenttool.HostNotFound, nil
	}
	for _, a := range addrs {
		if IsBlockedIP(a.IP) {
			return agenttool.HostBlocked, nil
		}
	}
	return agenttool.HostOK, nil
}
