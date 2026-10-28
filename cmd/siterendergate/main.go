package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"vozko/infra/siterendergate"
)

type domainList []string

func (d *domainList) String() string { return strings.Join(*d, ",") }

func (d *domainList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*d = append(*d, part)
		}
	}
	return nil
}

func serve(ctx context.Context, address string, handler http.Handler) {
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("siterendergate: %s: %v", address, err)
	}
}

func main() {
	var domains domainList
	flag.Var(&domains, "preview-domain", "domain whose hosts preview renders may reach (repeat or comma separate)")
	previewListen := flag.String("preview-listen", ":3128", "address of the preview only forward proxy")
	previewLoopback := flag.String("preview-loopback", "", "API address for preview hosts under localhost, for local development, for example host.docker.internal:4000")
	publicListen := flag.String("public-listen", ":3129", "address of the public reference forward proxy")
	devtoolsListen := flag.String("devtools-listen", ":9222", "address that relays DevTools to the renderer")
	devtoolsUpstream := flag.String("devtools-upstream", "", "renderer DevTools address, for example siterender:9222")
	devtoolsTokenFile := flag.String("devtools-token-file", "", "file holding the token every DevTools connection must carry as its first path segment")
	devtoolsTunnels := flag.Int("devtools-max-tunnels", 16, "most DevTools connections relayed at the same time")
	flag.Parse()
	if len(domains) == 0 {
		log.Fatal("siterendergate: -preview-domain is required")
	}
	if strings.TrimSpace(*devtoolsTokenFile) == "" {
		log.Fatal("siterendergate: -devtools-token-file is required")
	}
	token, err := os.ReadFile(*devtoolsTokenFile)
	if err != nil {
		log.Fatalf("siterendergate: the devtools token: %v", err)
	}
	relay, err := siterendergate.NewRelay(*devtoolsUpstream, strings.TrimSpace(string(token)), *devtoolsTunnels)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", *devtoolsListen)
	if err != nil {
		log.Fatalf("siterendergate: %v", err)
	}
	go func() {
		if err := relay.Serve(ctx, listener); err != nil {
			log.Fatalf("siterendergate: devtools relay: %v", err)
		}
	}()
	go serve(ctx, *publicListen, siterendergate.NewPublic())
	log.Printf("siterendergate: preview proxy on %s for %s, public proxy on %s, devtools %s to %s", *previewListen, domains.String(), *publicListen, *devtoolsListen, *devtoolsUpstream)
	serve(ctx, *previewListen, siterendergate.NewPreview(domains, *previewLoopback))
}
