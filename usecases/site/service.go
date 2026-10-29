package site_usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const (
	AutosaveWindow = 3 * time.Minute
	maxNameRunes   = 120
	maxLabelRunes  = 120
	MaxListLimit   = 100
)

type Deps struct {
	Store      Store
	Executor   Executor
	Previewer  Previewer
	References FormReferences
	Bindings   FormBindings
	Forms      FormsRetirement
	Retirement PublicationRetirement
	Published  PublishedRevisions
	Media      MediaLocator
	Numbers    WhatsAppNumbers
	Hostnames  HostnameLister
	Previews   PreviewCache
	Origins    sitepublish.Origins
	Now        func() time.Time
	NewID      func() string
}

type Service struct {
	store      Store
	executor   Executor
	previewer  Previewer
	references FormReferences
	bindings   FormBindings
	forms      FormsRetirement
	retirement PublicationRetirement
	published  PublishedRevisions
	media      MediaLocator
	numbers    WhatsAppNumbers
	hostnames  HostnameLister
	previews   PreviewCache
	origins    sitepublish.Origins
	now        func() time.Time
	newID      func() string
}

func NewService(d Deps) (*Service, error) {
	if d.Store == nil || d.Executor == nil || d.Previewer == nil || d.References == nil || d.Bindings == nil || d.Forms == nil || d.Retirement == nil || d.Published == nil || d.Media == nil ||
		d.Numbers == nil || d.Hostnames == nil || d.Previews == nil {
		return nil, ErrMissingDependency
	}
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	if d.NewID == nil {
		d.NewID = uuid.NewString
	}
	return &Service{store: d.Store, executor: d.Executor, previewer: d.Previewer, references: d.References, bindings: d.Bindings, forms: d.Forms, retirement: d.Retirement, published: d.Published, media: d.Media,
		numbers: d.Numbers, hostnames: d.Hostnames, previews: d.Previews, origins: d.Origins, now: d.Now, newID: d.NewID}, nil
}

func contentSHA(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func validName(name string) bool {
	trimmed := strings.TrimSpace(name)
	return trimmed != "" && utf8.RuneCountInString(trimmed) <= maxNameRunes
}

func encodeFiles(files map[string]site.File) (map[string]ManifestEntry, []Blob) {
	manifest := make(map[string]ManifestEntry, len(files))
	seen := map[string]bool{}
	var blobs []Blob
	for _, path := range slices.Sorted(maps.Keys(files)) {
		file := files[path]
		sha := contentSHA(file.Content)
		manifest[path] = ManifestEntry{SHA256: sha, Rev: file.Rev}
		if !seen[sha] {
			seen[sha] = true
			blobs = append(blobs, Blob{SHA256: sha, Bytes: []byte(file.Content)})
		}
	}
	return manifest, blobs
}

func newFileRevs(manifest, base map[string]ManifestEntry) []FileRev {
	var out []FileRev
	for _, path := range slices.Sorted(maps.Keys(manifest)) {
		entry := manifest[path]
		if old, ok := base[path]; ok && old == entry {
			continue
		}
		out = append(out, FileRev{Path: path, Rev: entry.Rev, SHA256: entry.SHA256})
	}
	return out
}

func newBlobs(blobs []Blob, base map[string]ManifestEntry) []Blob {
	known := map[string]bool{}
	for _, entry := range base {
		known[entry.SHA256] = true
	}
	out := make([]Blob, 0, len(blobs))
	for _, b := range blobs {
		if !known[b.SHA256] {
			out = append(out, b)
		}
	}
	return out
}

func (s *Service) source(ctx context.Context, p *Project, rev *Revision) (map[string]site.File, error) {
	shas := make([]string, 0, len(rev.Manifest))
	for _, entry := range rev.Manifest {
		shas = append(shas, entry.SHA256)
	}
	slices.Sort(shas)
	shas = slices.Compact(shas)
	blobs, err := s.store.Blobs(ctx, p.WorkspaceID, shas)
	if err != nil {
		return nil, err
	}
	files := make(map[string]site.File, len(rev.Manifest))
	for path, entry := range rev.Manifest {
		body, ok := blobs[entry.SHA256]
		if !ok || contentSHA(string(body)) != entry.SHA256 {
			return nil, fmt.Errorf("%w: %s", ErrRevisionDamaged, path)
		}
		files[path] = site.File{Content: string(body), Rev: entry.Rev}
	}
	return files, nil
}

func (s *Service) working(ctx context.Context, workspaceID, id string) (*View, error) {
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	p, err := s.store.GetProject(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	rev, err := s.store.GetRevision(ctx, workspaceID, p.ID, p.WorkingRevisionID)
	if err != nil {
		return nil, err
	}
	files, err := s.source(ctx, p, rev)
	if err != nil {
		return nil, err
	}
	return &View{Project: *p, Files: files, Revision: *rev}, nil
}

func editable(p Project) error {
	switch p.Status {
	case StatusArchived:
		return ErrProjectArchived
	case StatusSuspended:
		return ErrProjectSuspended
	}
	return nil
}

func (s *Service) editableAt(ctx context.Context, workspaceID, id string, expected int64) (*View, error) {
	if err := shared.RequireVersion(expected); err != nil {
		return nil, err
	}
	v, err := s.working(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	if err := editable(v.Project); err != nil {
		return nil, err
	}
	if err := shared.ExpectVersion(v.Project.Version, &expected); err != nil {
		return nil, err
	}
	live, err := s.liveRevision(ctx, v.Project)
	if err != nil {
		return nil, err
	}
	v.liveRevisionID = live
	v.Project.HasUnpublishedChanges = live != v.Project.WorkingRevisionID
	v.HasUnpublishedChanges = v.Project.HasUnpublishedChanges
	return v, nil
}

type revisionPlan struct {
	author Author
	kind   RevisionKind
	label  string
	steps  []site.StepPatch
}

func (s *Service) commit(ctx context.Context, base *View, files map[string]site.File, plan revisionPlan) (*View, error) {
	now := s.now()
	manifest, blobs := encodeFiles(files)
	replace := plan.kind == RevisionAutosave && base.Revision.Kind == RevisionAutosave &&
		base.Revision.Author == plan.author && now.Sub(base.Revision.CreatedAt) < AutosaveWindow
	rev := Revision{
		ID: s.newID(), WorkspaceID: base.Project.WorkspaceID, ProjectID: base.Project.ID, ParentID: base.Revision.ID,
		Manifest: manifest, Author: plan.author, Label: plan.label, Kind: plan.kind, CreatedAt: now, UpdatedAt: now,
	}
	if replace {
		rev.ID, rev.ParentID, rev.CreatedAt, rev.Label = base.Revision.ID, base.Revision.ParentID, base.Revision.CreatedAt, base.Revision.Label
	}
	p, err := s.store.Commit(ctx, Commit{
		WorkspaceID: base.Project.WorkspaceID, ProjectID: base.Project.ID, ExpectedVersion: base.Project.Version,
		Blobs: newBlobs(blobs, base.Revision.Manifest), Revision: rev, Replace: replace, FileRevs: newFileRevs(manifest, base.Revision.Manifest),
	})
	if err != nil {
		return nil, err
	}
	p.HasUnpublishedChanges = base.liveRevisionID != p.WorkingRevisionID
	return &View{Project: *p, Files: files, Revision: rev, HasUnpublishedChanges: p.HasUnpublishedChanges, Changes: site.Changes(base.Files, files, plan.steps), liveRevisionID: base.liveRevisionID}, nil
}

func newIssues(before, after []site.Issue) []site.Issue {
	known := map[site.Issue]int{}
	for _, issue := range before {
		known[issue]++
	}
	var out []site.Issue
	for _, issue := range after {
		if known[issue] > 0 {
			known[issue]--
			continue
		}
		out = append(out, issue)
	}
	return out
}

func introduced(base, next site.Project) []site.Issue {
	return newIssues(site.Validate(base), site.Validate(next))
}

func sameFiles(a, b map[string]site.File) bool {
	if len(a) != len(b) {
		return false
	}
	for path, file := range a {
		other, ok := b[path]
		if !ok || other.Content != file.Content {
			return false
		}
	}
	return true
}
