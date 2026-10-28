package netguard

import (
	"net"
	"time"
)

func NewDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second, Control: refuseBlocked}
}
