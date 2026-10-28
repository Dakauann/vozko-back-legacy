package sitebuilder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"vozko/domain/inlinemarkup"
	"vozko/domain/site"
	"vozko/domain/strictjson"
	"vozko/usecases/sitebuild"
)

const (
	DefaultExecuteTimeout = 15 * time.Second
	DefaultBuildTimeout   = 60 * time.Second
	maxReplyBytes         = 256 << 20
	maxErrorBytes         = 64 << 10
)

var assetPath = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*(/[A-Za-z0-9_][A-Za-z0-9_.-]*)*$`)

type Config struct {
	BaseURL        string
	Token          string
	ExecuteTimeout time.Duration
	BuildTimeout   time.Duration
	HTTPClient     *http.Client
}

type Client struct {
	base           string
	token          string
	executeTimeout time.Duration
	buildTimeout   time.Duration
	http           *http.Client
}

var _ sitebuild.Builder = (*Client)(nil)

func New(cfg Config) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("sitebuilder: the builder url must be an absolute http url")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("sitebuilder: the builder token is required")
	}
	c := &Client{base: u.String(), token: cfg.Token, executeTimeout: cfg.ExecuteTimeout, buildTimeout: cfg.BuildTimeout, http: cfg.HTTPClient}
	if c.executeTimeout <= 0 {
		c.executeTimeout = DefaultExecuteTimeout
	}
	if c.buildTimeout <= 0 {
		c.buildTimeout = DefaultBuildTimeout
	}
	if c.http == nil {
		c.http = &http.Client{Transport: http.DefaultTransport}
	}
	return c, nil
}

type executeWire struct {
	Project    site.Project                  `json:"project"`
	Operations []site.Operation              `json:"operations"`
	Texts      map[string][]inlinemarkup.Run `json:"texts"`
	Media      map[string]string             `json:"media"`
	Sections   map[string]string             `json:"sections"`
	JSX        map[string]string             `json:"jsx_sections"`
	Bases      map[string][]site.File        `json:"bases"`
}

type extractWire struct {
	Project site.Project           `json:"project"`
	Node    string                 `json:"node"`
	Rev     uint64                 `json:"rev,omitempty"`
	Bases   map[string][]site.File `json:"bases"`
}

type extractReply struct {
	Source  string                    `json:"source"`
	Failure *sitebuild.ExecuteFailure `json:"failure"`
}

type executeReply struct {
	Steps    []site.StepPatch           `json:"steps"`
	Splices  []sitebuild.Splice         `json:"splices"`
	Failures []sitebuild.ExecuteFailure `json:"failures"`
	Skipped  []site.Skip                `json:"skipped"`
}

type buildWire struct {
	Mode    sitebuild.Mode    `json:"mode"`
	Project site.Project      `json:"project"`
	Shell   sitebuild.Shell   `json:"shell"`
	Runtime map[string]string `json:"runtime"`
}

type assetWire struct {
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

type routeWire struct {
	Route       string `json:"route"`
	HTMLPath    string `json:"html_path"`
	Interactive bool   `json:"interactive"`
}

type buildReply struct {
	Assets        []assetWire              `json:"assets"`
	Imports       []site.Import            `json:"imports"`
	Routes        []routeWire              `json:"routes"`
	Errors        []sitebuild.BuildError   `json:"errors"`
	Warnings      []sitebuild.BuildWarning `json:"warnings"`
	PrerenderPath string                   `json:"prerender_path"`
}

type errorWire struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func orEmpty[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}
	return m
}

func malformed(reason string) error {
	return &sitebuild.BuilderError{Kind: sitebuild.ErrBuilderReply, Message: reason}
}

func (c *Client) Execute(ctx context.Context, req sitebuild.ExecuteRequest) (sitebuild.ExecuteResult, error) {
	texts := map[string][]inlinemarkup.Run{}
	for index, runs := range req.Texts {
		if runs == nil {
			runs = []inlinemarkup.Run{}
		}
		texts[strconv.Itoa(index)] = runs
	}
	wire := executeWire{
		Project: req.Project, Operations: req.Operations, Texts: texts,
		Media: orEmpty(req.Media), Sections: orEmpty(req.Sections), JSX: orEmpty(req.JSXSections), Bases: orEmpty(req.Bases),
	}
	raw, err := c.post(ctx, "/execute", wire, c.executeTimeout)
	if err != nil {
		return sitebuild.ExecuteResult{}, err
	}
	var reply executeReply
	if err := strictjson.Decode(raw, &reply); err != nil {
		return sitebuild.ExecuteResult{}, malformed(err.Error())
	}
	if err := checkExecute(reply, len(req.Operations)); err != nil {
		return sitebuild.ExecuteResult{}, err
	}
	return sitebuild.ExecuteResult{Steps: reply.Steps, Splices: reply.Splices, Failures: reply.Failures, Skipped: reply.Skipped}, nil
}

func checkExecute(reply executeReply, operations int) error {
	if (len(reply.Steps) == 0) == (len(reply.Failures) == 0) {
		return malformed("an execution has either steps or failures")
	}
	inRange := func(index int) bool { return index >= 0 && index < operations }
	last := -1
	for _, step := range reply.Steps {
		if !step.Op.Known() || !inRange(step.Index) || step.Index <= last {
			return malformed("a step does not match the batch")
		}
		last = step.Index
	}
	for _, s := range reply.Splices {
		if !inRange(s.Index) || s.Start < 0 || s.End < s.Start || s.Length < 0 {
			return malformed("a splice does not match the batch")
		}
	}
	codes := site.Codes()
	for _, f := range reply.Failures {
		if !inRange(f.Index) || !slices.Contains(codes, f.Code) {
			return malformed("a failure does not match the batch")
		}
	}
	for _, s := range reply.Skipped {
		if !inRange(s.Index) || !inRange(s.After) || s.After >= s.Index {
			return malformed("a skip does not match the batch")
		}
	}
	return nil
}

func (c *Client) Extract(ctx context.Context, req sitebuild.ExtractRequest) (sitebuild.ExtractResult, error) {
	raw, err := c.post(ctx, "/extract", extractWire{Project: req.Project, Node: req.Node, Rev: req.Rev, Bases: orEmpty(req.Bases)}, c.executeTimeout)
	if err != nil {
		return sitebuild.ExtractResult{}, err
	}
	var reply extractReply
	if err := strictjson.Decode(raw, &reply); err != nil {
		return sitebuild.ExtractResult{}, malformed(err.Error())
	}
	if (reply.Source == "") == (reply.Failure == nil) {
		return sitebuild.ExtractResult{}, malformed("an extraction has either a source or a failure")
	}
	if reply.Failure != nil && !slices.Contains(site.Codes(), reply.Failure.Code) {
		return sitebuild.ExtractResult{}, malformed("the extraction failed with an unknown code")
	}
	return sitebuild.ExtractResult{Source: reply.Source, Failure: reply.Failure}, nil
}

func (c *Client) Build(ctx context.Context, req sitebuild.BuildRequest) (sitebuild.BuildResult, error) {
	shell := req.Shell
	shell.Forms = orEmpty(shell.Forms)
	shell.Pages = orEmpty(shell.Pages)
	shell.Integrations = orEmpty(shell.Integrations)
	raw, err := c.post(ctx, "/build", buildWire{Mode: req.Mode, Project: req.Project, Shell: shell, Runtime: orEmpty(req.Runtime)}, c.buildTimeout)
	if err != nil {
		return sitebuild.BuildResult{}, err
	}
	var reply buildReply
	if err := strictjson.Decode(raw, &reply); err != nil {
		return sitebuild.BuildResult{}, malformed(err.Error())
	}
	if err := checkBuild(reply); err != nil {
		return sitebuild.BuildResult{}, err
	}
	out := sitebuild.BuildResult{Imports: reply.Imports, Errors: reply.Errors, Warnings: reply.Warnings, PrerenderPath: reply.PrerenderPath}
	for _, a := range reply.Assets {
		out.Assets = append(out.Assets, sitebuild.Asset{Path: a.Path, ContentType: a.ContentType, Body: a.Body})
	}
	for _, r := range reply.Routes {
		out.Routes = append(out.Routes, sitebuild.RouteOutput{Route: r.Route, HTMLPath: r.HTMLPath, Interactive: r.Interactive})
	}
	return out, nil
}

func checkBuild(reply buildReply) error {
	for _, e := range reply.Errors {
		if e.Code != "" && !slices.Contains(site.Codes(), e.Code) {
			return malformed("a build error carries an unknown code")
		}
	}
	if len(reply.Errors) > 0 {
		if len(reply.Assets) > 0 || len(reply.Routes) > 0 || reply.PrerenderPath != "" {
			return malformed("a failed build carries no output")
		}
		return nil
	}
	if len(reply.Routes) == 0 {
		return malformed("a build has at least one route")
	}
	paths := map[string]bool{}
	for _, a := range reply.Assets {
		if !assetPath.MatchString(a.Path) || strings.Contains(a.Path, "..") || paths[a.Path] || strings.TrimSpace(a.ContentType) == "" {
			return malformed("the asset " + a.Path + " is not valid")
		}
		paths[a.Path] = true
	}
	for _, r := range reply.Routes {
		if !site.ValidRoutePath(r.Route) || !paths[r.HTMLPath] {
			return malformed("the route " + r.Route + " has no page")
		}
	}
	if reply.PrerenderPath != "" && !paths[reply.PrerenderPath] {
		return malformed("the prerender page is missing")
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body any, timeout time.Duration) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("sitebuilder: encode request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return nil, &sitebuild.BuilderError{Kind: sitebuild.ErrBuilderUnavailable, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &sitebuild.BuilderError{Kind: sitebuild.ErrBuilderTimeout, Message: err.Error()}
		}
		return nil, &sitebuild.BuilderError{Kind: sitebuild.ErrBuilderUnavailable, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &sitebuild.BuilderError{Kind: sitebuild.ErrBuilderTimeout, Message: err.Error()}
		}
		return nil, &sitebuild.BuilderError{Kind: sitebuild.ErrBuilderUnavailable, Message: err.Error()}
	}
	if len(raw) > maxReplyBytes {
		return nil, malformed("the reply is too large")
	}
	return raw, nil
}

func statusError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
	var detail errorWire
	_ = json.Unmarshal(raw, &detail)
	kind := sitebuild.ErrBuilderUnavailable
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusRequestEntityTooLarge:
		kind = sitebuild.ErrBuilderRefused
	case http.StatusServiceUnavailable:
		kind = sitebuild.ErrBuilderBusy
	case http.StatusGatewayTimeout:
		kind = sitebuild.ErrBuilderTimeout
	}
	return &sitebuild.BuilderError{Kind: kind, Status: resp.StatusCode, Code: detail.Code, Message: detail.Message}
}
