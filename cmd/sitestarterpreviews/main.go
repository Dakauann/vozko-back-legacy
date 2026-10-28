package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vozko/infra/sitebuilder"
	siterenderinfra "vozko/infra/siterender"
	"vozko/infra/siterendergate"
	"vozko/infra/siteruntime"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siterender"
)

type loopback struct{}

func (loopback) Allow(_ context.Context, host string) bool {
	return host == "127.0.0.1"
}

func serve(result sitebuild.BuildResult) (string, func(), error) {
	bodies := map[string]sitebuild.Asset{}
	for _, a := range result.Assets {
		bodies["/"+a.Path] = a
	}
	for _, r := range result.Routes {
		bodies[r.Route] = bodies["/"+r.HTMLPath]
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", asset.ContentType)
		_, _ = w.Write(asset.Body)
	})}
	go func() { _ = server.Serve(listener) }()
	return "http://" + listener.Addr().String(), func() { _ = server.Close() }, nil
}

func main() {
	builderURL := flag.String("builder", "http://127.0.0.1:8080", "site builder address")
	token := flag.String("token", os.Getenv("SITES_BUILDER_TOKEN"), "site builder bearer token")
	devtools := flag.String("devtools", "127.0.0.1:9333", "DevTools host and port of a headless Chromium")
	out := flag.String("out", filepath.Join("usecases", "site", "starters"), "starters folder that receives preview.jpg files")
	flag.Parse()

	client, err := sitebuilder.New(sitebuilder.Config{BaseURL: *builderURL, Token: *token})
	if err != nil {
		log.Fatal(err)
	}
	browser, err := relayedBrowser(*devtools)
	if err != nil {
		log.Fatal(err)
	}
	runtime, err := siteruntime.Files()
	if err != nil {
		log.Fatal(err)
	}
	starters, err := site_usecase.StarterSources()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	for _, starter := range starters {
		result, err := client.Build(ctx, sitebuild.BuildRequest{
			Mode: sitebuild.ModePreview, Project: starter.Project(), Runtime: runtime,
			Shell: sitebuild.Shell{API: "http://localhost", Forms: map[string]string{}, Pages: map[string]sitebuild.PageMeta{}, Integrations: map[string]string{}},
		})
		if err != nil {
			log.Fatalf("%s/%s: %v", starter.Stack, starter.Key, err)
		}
		if len(result.Errors) > 0 {
			log.Fatalf("%s/%s: %+v", starter.Stack, starter.Key, result.Errors)
		}
		origin, stop, err := serve(result)
		if err != nil {
			log.Fatal(err)
		}
		shot, err := browser.Screenshot(ctx, siterender.ScreenshotJob{URL: origin + "/", Viewport: siterender.Desktop, Policy: loopback{}})
		stop()
		if err != nil {
			log.Fatalf("%s/%s: %v", starter.Stack, starter.Key, err)
		}
		target := filepath.Join(*out, string(starter.Stack), starter.Key, site_usecase.StarterPreviewFile)
		if err := os.WriteFile(target, shot, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println(strings.ReplaceAll(target, "\\", "/"), len(shot), "bytes")
	}
}

func relayedBrowser(devtools string) (*siterenderinfra.Client, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(secret)
	relay, err := siterendergate.NewRelay(devtools, token, 4)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() { _ = relay.Serve(context.Background(), listener) }()
	return siterenderinfra.New(siterenderinfra.Config{Endpoint: "http://" + listener.Addr().String(), Token: token})
}
