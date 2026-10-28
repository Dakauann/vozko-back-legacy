package siterendergate

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var relayToken = strings.Repeat("t", 40)

type echoUpstream struct {
	address string
	dials   atomic.Int32
}

func startEcho(t *testing.T) *echoUpstream {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	echo := &echoUpstream{address: listener.Addr().String()}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			echo.dials.Add(1)
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return echo
}

func startRelay(t *testing.T, upstream string, maxTunnels int) string {
	t.Helper()
	relay, err := NewRelay(upstream, relayToken, maxTunnels)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = relay.Serve(ctx, listener) }()
	return listener.Addr().String()
}

func send(t *testing.T, address, request string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprint(conn, request)
	return conn, bufio.NewReader(conn)
}

func TestRelayStripsAValidTokenAndCopiesBytesBothWays(t *testing.T) {
	echo := startEcho(t)
	address := startRelay(t, echo.address, 16)
	cases := map[string]string{
		"/" + relayToken + "/json/version":             "/json/version",
		"/" + relayToken + "/devtools/browser/abc?x=1": "/devtools/browser/abc?x=1",
		"/" + relayToken:                                  "/",
		"/" + relayToken + "?probe=1":                     "/?probe=1",
		"/" + relayToken + "/devtools/page/" + relayToken: "/devtools/page/" + relayToken,
	}
	for sent, want := range cases {
		conn, reader := send(t, address, "GET "+sent+" HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\nping")
		line, err := reader.ReadString('\n')
		if err != nil || line != "GET "+want+" HTTP/1.1\r\n" {
			t.Fatalf("%s: upstream saw %q %v, want the path %s", sent, line, err, want)
		}
		rest := make([]byte, len("Host: 127.0.0.1\r\n\r\nping"))
		if _, err := io.ReadFull(reader, rest); err != nil || string(rest) != "Host: 127.0.0.1\r\n\r\nping" {
			t.Fatalf("%s: the rest of the stream %q %v", sent, rest, err)
		}
		_ = conn.Close()
	}
}

func TestRelayRefusesConnectionsWithoutTheToken(t *testing.T) {
	echo := startEcho(t)
	address := startRelay(t, echo.address, 16)
	for _, request := range []string{
		"GET /json/version HTTP/1.1\r\n\r\n",
		"GET /" + strings.Repeat("x", 40) + "/json/version HTTP/1.1\r\n\r\n",
		"GET /" + relayToken + "x/json/version HTTP/1.1\r\n\r\n",
		"GET /" + relayToken[:39] + "/json/version HTTP/1.1\r\n\r\n",
		"GET //" + relayToken + "/json/version HTTP/1.1\r\n\r\n",
		"GET * HTTP/1.1\r\n\r\n",
		"garbage\r\n\r\n",
		"\x16\x03\x01\x02\x00\x01\x00\x01\xfc\x03\x03\r\n",
	} {
		_, reader := send(t, address, request)
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(line, "HTTP/1.1 403") {
			t.Fatalf("%q: reply %q, want 403", request, line)
		}
	}
	if n := echo.dials.Load(); n != 0 {
		t.Fatalf("refused connections reached the upstream %d times", n)
	}
}

func TestRelayRefusesAnOversizedRequestLine(t *testing.T) {
	echo := startEcho(t)
	address := startRelay(t, echo.address, 16)
	_, reader := send(t, address, "GET /"+relayToken+"/"+strings.Repeat("a", 9000)+" HTTP/1.1\r\n\r\n")
	line, _ := reader.ReadString('\n')
	if !strings.HasPrefix(line, "HTTP/1.1 403") || echo.dials.Load() != 0 {
		t.Fatalf("reply %q, dials %d", line, echo.dials.Load())
	}
}

func TestRelayCapsConcurrentTunnels(t *testing.T) {
	echo := startEcho(t)
	address := startRelay(t, echo.address, 1)
	held, reader := send(t, address, "GET /"+relayToken+"/devtools/browser/a HTTP/1.1\r\n\r\n")
	if line, err := reader.ReadString('\n'); err != nil || !strings.HasPrefix(line, "GET /devtools/browser/a") {
		t.Fatalf("first tunnel %q %v", line, err)
	}

	_, refused := send(t, address, "GET /"+relayToken+"/devtools/browser/b HTTP/1.1\r\n\r\n")
	if line, _ := refused.ReadString('\n'); !strings.HasPrefix(line, "HTTP/1.1 503") {
		t.Fatalf("second tunnel reply %q, want 503", line)
	}

	_ = held.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, again := send(t, address, "GET /"+relayToken+"/devtools/browser/c HTTP/1.1\r\n\r\n")
		line, _ := again.ReadString('\n')
		if strings.HasPrefix(line, "GET /devtools/browser/c") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the slot was never released, last reply %q", line)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNewRelayRefusesWeakSettings(t *testing.T) {
	cases := map[string]struct {
		upstream string
		token    string
		max      int
	}{
		"short token":        {"siterender:9222", strings.Repeat("t", 31), 4},
		"no upstream":        {" ", relayToken, 4},
		"no tunnels":         {"siterender:9222", relayToken, 0},
		"token with a slash": {"siterender:9222", strings.Repeat("t", 31) + "/", 4},
	}
	for name, tc := range cases {
		if _, err := NewRelay(tc.upstream, tc.token, tc.max); err == nil {
			t.Fatalf("%s: NewRelay accepted it", name)
		}
	}
}
