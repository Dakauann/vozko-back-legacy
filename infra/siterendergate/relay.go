package siterendergate

import (
	"bufio"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

const (
	MinRelayTokenLength = 32
	requestLineLimit    = 4096
	requestLineTimeout  = 10 * time.Second
	refusalDrainLimit   = 64 << 10
	refusalDrainTimeout = 2 * time.Second
)

type Relay struct {
	upstream string
	token    []byte
	slots    chan struct{}
}

func NewRelay(upstream, token string, maxTunnels int) (*Relay, error) {
	if strings.TrimSpace(upstream) == "" {
		return nil, errors.New("siterendergate: the devtools upstream is required")
	}
	if !ValidRelayToken(token) {
		return nil, fmt.Errorf("siterendergate: the devtools token needs at least %d characters from A-Z, a-z, 0-9, dot, underscore, tilde and hyphen", MinRelayTokenLength)
	}
	if maxTunnels < 1 {
		return nil, errors.New("siterendergate: the devtools relay needs at least one tunnel")
	}
	return &Relay{upstream: upstream, token: []byte(token), slots: make(chan struct{}, maxTunnels)}, nil
}

func ValidRelayToken(token string) bool {
	if len(token) < MinRelayTokenLength {
		return false
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.', r == '_', r == '~':
		default:
			return false
		}
	}
	return true
}

func (r *Relay) Serve(ctx context.Context, listener net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		client, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		select {
		case r.slots <- struct{}{}:
			go func() {
				defer func() { <-r.slots }()
				r.handle(ctx, client)
			}()
		default:
			log.Printf("siterendergate: devtools relay is full, refused %s", client.RemoteAddr())
			go refuseConn(client, "503 Service Unavailable")
		}
	}
}

func (r *Relay) handle(ctx context.Context, client net.Conn) {
	reader := bufio.NewReaderSize(client, requestLineLimit)
	_ = client.SetReadDeadline(time.Now().Add(requestLineTimeout))
	line, err := reader.ReadSlice('\n')
	if err != nil {
		refuseConn(client, "403 Forbidden")
		return
	}
	rewritten, ok := r.authorize(string(line))
	if !ok {
		log.Printf("siterendergate: devtools relay refused %s without a valid token", client.RemoteAddr())
		refuseConn(client, "403 Forbidden")
		return
	}
	_ = client.SetReadDeadline(time.Time{})
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	var d net.Dialer
	target, err := d.DialContext(dialCtx, "tcp", r.upstream)
	cancel()
	if err != nil {
		log.Printf("siterendergate: devtools upstream %s unreachable: %v", r.upstream, err)
		refuseConn(client, "502 Bad Gateway")
		return
	}
	if _, err := io.WriteString(target, rewritten); err != nil {
		_ = target.Close()
		_ = client.Close()
		return
	}
	pipe(primedConn{Conn: client, reader: reader}, target)
}

func (r *Relay) authorize(line string) (string, bool) {
	parts := strings.Split(strings.TrimRight(line, "\r\n"), " ")
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "/") || !strings.HasPrefix(parts[2], "HTTP/") {
		return "", false
	}
	target := parts[1][1:]
	segment, rest := target, ""
	if cut := strings.IndexAny(target, "/?"); cut >= 0 {
		segment, rest = target[:cut], target[cut:]
	}
	if subtle.ConstantTimeCompare([]byte(segment), r.token) != 1 {
		return "", false
	}
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	return parts[0] + " " + rest + " " + parts[2] + "\r\n", true
}

func refuseConn(conn net.Conn, status string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(refusalDrainTimeout))
	_, _ = io.WriteString(conn, "HTTP/1.1 "+status+"\r\n"+RefusalHeader+": refused\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
	if tcp, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = tcp.CloseWrite()
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(conn, refusalDrainLimit))
}

type primedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c primedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c primedConn) CloseWrite() error {
	if tcp, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return tcp.CloseWrite()
	}
	return c.Conn.Close()
}
