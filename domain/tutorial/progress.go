package tutorial

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

const MaxStep = 50

var (
	ErrUserRequired      = errors.New("tutorial: progress needs the person")
	ErrWorkspaceRequired = errors.New("tutorial: checklists need their workspace")
	ErrInvalidID         = errors.New("tutorial: the id must be area.name")
	ErrInvalidProgress   = errors.New("tutorial: unknown status or step out of range")
)

var idPattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,39}(\.[a-z][a-zA-Z0-9]{0,39}){1,2}$`)

type Status string

const (
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusDismissed  Status = "dismissed"
)

func (s Status) Valid() bool {
	return s == StatusInProgress || s == StatusCompleted || s == StatusDismissed
}

type Progress struct {
	TutorialID string
	Status     Status
	Step       int
	UpdatedAt  time.Time
}

func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

func RequireUser(userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrUserRequired
	}
	return nil
}

func NewProgress(id string, status Status, step int, at time.Time) (Progress, error) {
	if !ValidID(id) {
		return Progress{}, ErrInvalidID
	}
	if !status.Valid() || step < 0 || step > MaxStep {
		return Progress{}, ErrInvalidProgress
	}
	return Progress{TutorialID: id, Status: status, Step: step, UpdatedAt: at}, nil
}

func (p Progress) After(previous Progress, found bool) Progress {
	if !found || previous.Status != StatusCompleted || p.Status == StatusCompleted {
		return p
	}
	return Progress{TutorialID: p.TutorialID, Status: StatusCompleted, Step: previous.Step, UpdatedAt: p.UpdatedAt}
}

type ProgressRepository interface {
	List(ctx context.Context, userID string) ([]Progress, error)
	Find(ctx context.Context, userID, tutorialID string) (Progress, bool, error)
	Save(ctx context.Context, userID string, progress Progress) error
	Delete(ctx context.Context, userID, tutorialID string) error
}
