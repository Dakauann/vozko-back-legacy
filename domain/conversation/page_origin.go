package conversation

import (
	"context"
	"crypto/rand"
	"math/big"
	"regexp"
	"strings"
	"time"

	"vozko/domain/shared"
)

const (
	PageReferenceLength   = 5
	pageReferenceAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var pageReferencePattern = regexp.MustCompile(`(?i)\(\s*ref\s+([A-HJ-NP-Z2-9]{5})\s*\)`)

func NewPageReference() (string, error) {
	limit := big.NewInt(int64(len(pageReferenceAlphabet)))
	var b strings.Builder
	for range PageReferenceLength {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		b.WriteByte(pageReferenceAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func PrefilledText(text, code string) string {
	reference := "(ref " + code + ")"
	text = strings.TrimSpace(text)
	if text == "" {
		return reference
	}
	return text + " " + reference
}

func FindPageReference(text string) (string, bool) {
	match := pageReferencePattern.FindStringSubmatch(text)
	if match == nil {
		return "", false
	}
	return strings.ToUpper(match[1]), true
}

type PageOrigin struct {
	EntryID     string           `json:"entryId"`
	EntryType   shared.EntryType `json:"entryType"`
	WorkspaceID string           `json:"-"`
	ProjectID   string           `json:"projectId"`
	Route       string           `json:"route"`
	Reference   string           `json:"reference"`
	ArrivedAt   time.Time        `json:"arrivedAt"`
}

func (o PageOrigin) Complete() bool {
	for _, v := range []string{o.EntryID, string(o.EntryType), o.WorkspaceID, o.ProjectID, o.Reference} {
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}

type PageOriginView struct {
	ProjectID   string    `json:"projectId"`
	ProjectName string    `json:"projectName,omitempty"`
	Route       string    `json:"route"`
	URL         string    `json:"url,omitempty"`
	Reference   string    `json:"reference"`
	ArrivedAt   time.Time `json:"arrivedAt"`
}

type PageProject struct {
	Name string
	Live bool
}

type PageOriginReader interface {
	PageOrigin(ctx context.Context, entryID string, entryType shared.EntryType) (*PageOriginView, error)
}

type PageReferenceTarget struct {
	WorkspaceID string
	ProjectID   string
	Route       string
	NumberRef   string
}

type PageOriginRepository interface {
	Claim(ctx context.Context, origin PageOrigin) (bool, error)
	Get(ctx context.Context, entryID string, entryType shared.EntryType) (*PageOrigin, error)
}

type PageReferences interface {
	Target(ctx context.Context, code string) (*PageReferenceTarget, error)
}
