package netguard

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestNewDialerRefusesBlockedAddressesAtConnectTime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	_, err = NewDialer(time.Second).DialContext(context.Background(), "tcp", listener.Addr().String())
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("dial to loopback: %v", err)
	}
}
