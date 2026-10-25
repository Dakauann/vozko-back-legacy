package netguard

import (
	"context"
	"errors"
	"net"
	"testing"

	"vozko/domain/agenttool"
)

type scriptedResolver struct {
	addrs []net.IPAddr
	err   error
}

func (s scriptedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return s.addrs, s.err }

func TestHostVerdictsTellBlockedMissingAndUnavailableApart(t *testing.T) {
	cases := []struct {
		name     string
		host     string
		resolver scriptedResolver
		want     agenttool.HostVerdict
		failing  bool
	}{
		{"public", "api.test", scriptedResolver{addrs: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}, agenttool.HostOK, false},
		{"rebinding", "api.test", scriptedResolver{addrs: []net.IPAddr{{IP: net.ParseIP("10.0.0.7")}}}, agenttool.HostBlocked, false},
		{"metadata literal", "169.254.169.254", scriptedResolver{}, agenttool.HostBlocked, false},
		{"missing", "nope.test", scriptedResolver{err: &net.DNSError{Err: "no such host", IsNotFound: true}}, agenttool.HostNotFound, false},
		{"dns down", "api.test", scriptedResolver{err: errors.New("i/o timeout")}, agenttool.HostOK, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewHostVerdicts(tc.resolver).ClassifyHost(context.Background(), tc.host)
			if (err != nil) != tc.failing || got != tc.want {
				t.Fatalf("got %v err %v", got, err)
			}
		})
	}
}
