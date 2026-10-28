package siterendergate

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"vozko/domain/hostname"
	"vozko/infra/netguard"
)

const (
	dialTimeout     = 10 * time.Second
	tunnelLifetime  = 3 * time.Minute
	upstreamTimeout = 60 * time.Second
)

const RefusalHeader = "X-Site-Gateway"

func refuse(w http.ResponseWriter, message string, status int) {
	w.Header().Set(RefusalHeader, "refused")
	http.Error(w, message, status)
}

var hopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

type Proxy struct {
	Allow    func(host string) bool
	Dial     func(ctx context.Context, network, address string) (net.Conn, error)
	Loopback string

	once      sync.Once
	transport *http.Transport
}

func normalHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func PreviewRule(domains []string) func(host string) bool {
	var allowed []string
	for _, d := range domains {
		if d = normalHost(d); d != "" {
			allowed = append(allowed, d)
		}
	}
	return func(host string) bool {
		host = normalHost(host)
		if host == "" {
			return false
		}
		for _, d := range allowed {
			if host == d || strings.HasSuffix(host, "."+d) {
				return true
			}
		}
		return false
	}
}

func anyHost(host string) bool {
	return normalHost(host) != ""
}

func guardedDial(ctx context.Context, network, address string) (net.Conn, error) {
	return netguard.NewDialer(dialTimeout).DialContext(ctx, network, address)
}

func NewPreview(domains []string, loopback string) *Proxy {
	return &Proxy{Allow: PreviewRule(domains), Dial: guardedDial, Loopback: loopback}
}

func NewPublic() *Proxy {
	return &Proxy{Allow: anyHost, Dial: guardedDial}
}

func webPort(port string, connect bool) bool {
	if port == "" {
		return !connect
	}
	return port == "80" || port == "443"
}

func (p *Proxy) permitted(hostport string, connect bool) (string, bool) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = hostport, ""
	}
	if p.Allow == nil || !p.Allow(host) {
		return "", false
	}
	if hostname.Loopback(host) {
		return p.Loopback, p.Loopback != ""
	}
	if !webPort(port, connect) {
		return "", false
	}
	if port == "" {
		port = "80"
	}
	return net.JoinHostPort(host, port), true
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.tunnel(w, r)
		return
	}
	if !r.URL.IsAbs() || r.URL.Scheme != "http" {
		http.Error(w, "only proxy requests are accepted", http.StatusBadRequest)
		return
	}
	p.forward(w, r)
}

func (p *Proxy) tunnel(w http.ResponseWriter, r *http.Request) {
	address, ok := p.permitted(r.Host, true)
	if !ok {
		log.Printf("siterendergate: refused CONNECT %s", r.Host)
		refuse(w, "destination not allowed", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), dialTimeout)
	upstream, err := p.dial(ctx, "tcp", address)
	cancel()
	if err != nil {
		log.Printf("siterendergate: CONNECT %s failed: %v", address, err)
		refuse(w, "destination unreachable", http.StatusBadGateway)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "tunnelling unsupported", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	deadline := time.Now().Add(tunnelLifetime)
	_ = client.SetDeadline(deadline)
	_ = upstream.SetDeadline(deadline)
	if buffered.Reader.Buffered() > 0 {
		pending := make([]byte, buffered.Reader.Buffered())
		if _, err := io.ReadFull(buffered.Reader, pending); err == nil {
			_, _ = upstream.Write(pending)
		}
	}
	pipe(client, upstream)
}

func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	copyThenClose := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if tcp, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = tcp.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	go copyThenClose(a, b)
	go copyThenClose(b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}

func (p *Proxy) dial(ctx context.Context, network, address string) (net.Conn, error) {
	if p.Loopback != "" && address == p.Loopback {
		d := net.Dialer{Timeout: dialTimeout}
		return d.DialContext(ctx, network, address)
	}
	return p.Dial(ctx, network, address)
}

func (p *Proxy) roundTripper() *http.Transport {
	p.once.Do(func() {
		p.transport = &http.Transport{
			DialContext:           p.dial,
			Proxy:                 nil,
			MaxIdleConns:          32,
			IdleConnTimeout:       30 * time.Second,
			ResponseHeaderTimeout: upstreamTimeout,
		}
	})
	return p.transport
}

func (p *Proxy) forward(w http.ResponseWriter, r *http.Request) {
	address, ok := p.permitted(r.URL.Host, false)
	if !ok {
		log.Printf("siterendergate: refused %s %s", r.Method, r.URL.Host)
		refuse(w, "destination not allowed", http.StatusForbidden)
		return
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.URL.Host = address
	out.Host = r.URL.Hostname()
	for _, h := range hopHeaders {
		out.Header.Del(h)
	}
	resp, err := p.roundTripper().RoundTrip(out)
	if err != nil {
		log.Printf("siterendergate: %s %s failed: %v", r.Method, address, err)
		refuse(w, "destination unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, h := range hopHeaders {
		resp.Header.Del(h)
	}
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
