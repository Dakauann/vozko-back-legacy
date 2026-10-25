package agentscenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/tracepayload"
)

const (
	MaxMessages  = agent.MaxSimulationHistory
	MaxMockBytes = tracepayload.MaxBytes
	MaxNameRunes = 120
	MaxRubric    = 2000
	RunRetention = 180 * 24 * time.Hour
)

var (
	ErrNotFound     = errors.New("agent scenario: not found")
	ErrInvalid      = errors.New("agent scenario: invalid")
	ErrTooLong      = fmt.Errorf("agent scenario: more than %d messages", MaxMessages)
	ErrMockTooLarge = fmt.Errorf("agent scenario: a mock is larger than %d bytes", MaxMockBytes)
)

type Kind string

const (
	KindToolCalled        Kind = "tool_called"
	KindToolNotCalled     Kind = "tool_not_called"
	KindToolOutputMatches Kind = "tool_output_matches"
	KindReplyMatches      Kind = "reply_matches"
	KindReplyJudged       Kind = "reply_judged"
)

type Match string

const (
	MatchExact    Match = "exact"
	MatchSubset   Match = "subset"
	MatchRegex    Match = "regex"
	MatchContains Match = "contains"
)

type Message struct {
	Content string `json:"content"`
}

type Mock struct {
	Result  interface{} `json:"result"`
	IsError bool        `json:"isError"`
}

type Assertion struct {
	Kind      Kind                   `json:"kind"`
	ToolID    string                 `json:"toolId,omitempty"`
	Match     Match                  `json:"match,omitempty"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
	Path      string                 `json:"path,omitempty"`
	Value     string                 `json:"value,omitempty"`
	Rubric    string                 `json:"rubric,omitempty"`
}

type RunSummary struct {
	ID         string    `json:"id"`
	Passed     bool      `json:"passed"`
	FinishedAt time.Time `json:"finishedAt"`
}

type Scenario struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	AgentID     string          `json:"agentId"`
	Name        string          `json:"name"`
	Messages    []Message       `json:"messages"`
	Mocks       map[string]Mock `json:"mocks"`
	Assertions  []Assertion     `json:"assertions"`
	SourceRunID string          `json:"sourceRunId,omitempty"`
	CreatedBy   string          `json:"createdBy,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	Last        *RunSummary     `json:"last"`
	Previous    *RunSummary     `json:"previous"`
}

type AssertionResult struct {
	Index  int    `json:"index"`
	Kind   Kind   `json:"kind"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}

type Turn struct {
	Message string         `json:"message"`
	Replies []string       `json:"replies"`
	Calls   []ObservedCall `json:"calls"`
	RunID   string         `json:"runId,omitempty"`
}

type Run struct {
	ID          string            `json:"id"`
	ScenarioID  string            `json:"scenarioId"`
	WorkspaceID string            `json:"workspaceId"`
	AgentID     string            `json:"agentId"`
	Passed      bool              `json:"passed"`
	Results     []AssertionResult `json:"results"`
	Turns       []Turn            `json:"turns"`
	RunIDs      []string          `json:"runIds"`
	Error       string            `json:"error,omitempty"`
	StartedAt   time.Time         `json:"startedAt"`
	FinishedAt  time.Time         `json:"finishedAt"`
}

func (r Run) Summary() *RunSummary {
	return &RunSummary{ID: r.ID, Passed: r.Passed, FinishedAt: r.FinishedAt}
}

func (s Scenario) Check() error {
	if name := strings.TrimSpace(s.Name); name == "" || len([]rune(name)) > MaxNameRunes {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	if len(s.Messages) > MaxMessages {
		return ErrTooLong
	}
	if len(s.Messages) == 0 {
		return fmt.Errorf("%w: messages", ErrInvalid)
	}
	for _, m := range s.Messages {
		if strings.TrimSpace(m.Content) == "" {
			return fmt.Errorf("%w: empty message", ErrInvalid)
		}
	}
	for toolID, mock := range s.Mocks {
		if strings.TrimSpace(toolID) == "" {
			return fmt.Errorf("%w: mock without tool", ErrInvalid)
		}
		encoded, err := json.Marshal(mock.Result)
		if err != nil {
			return fmt.Errorf("%w: mock", ErrInvalid)
		}
		if len(encoded) > MaxMockBytes {
			return ErrMockTooLarge
		}
	}
	if len(s.Assertions) == 0 {
		return fmt.Errorf("%w: assertions", ErrInvalid)
	}
	for i, a := range s.Assertions {
		if err := a.check(); err != nil {
			return fmt.Errorf("%w: assertion %d: %s", ErrInvalid, i, err.Error())
		}
	}
	return nil
}

func (a Assertion) check() error {
	switch a.Kind {
	case KindToolCalled:
		if a.ToolID == "" {
			return errors.New("tool")
		}
		if len(a.Arguments) > 0 && a.Match != MatchExact && a.Match != MatchSubset && a.Match != MatchRegex {
			return errors.New("match")
		}
		if a.Match == MatchRegex {
			return regexArguments(a.Arguments)
		}
	case KindToolNotCalled:
		if a.ToolID == "" {
			return errors.New("tool")
		}
	case KindToolOutputMatches:
		if a.ToolID == "" || !agenttool.ValidOutputPath(a.Path) {
			return errors.New("tool and path")
		}
		return textRule(a, MatchExact)
	case KindReplyMatches:
		if strings.TrimSpace(a.Value) == "" {
			return errors.New("value")
		}
		return textRule(a, MatchContains)
	case KindReplyJudged:
		if rubric := strings.TrimSpace(a.Rubric); rubric == "" || len([]rune(rubric)) > MaxRubric {
			return errors.New("rubric")
		}
	default:
		return errors.New("kind")
	}
	return nil
}

func textRule(a Assertion, plain Match) error {
	if a.Match != plain && a.Match != MatchRegex {
		return errors.New("match")
	}
	if a.Match == MatchRegex {
		if _, err := regexp.Compile(a.Value); err != nil {
			return errors.New("regex")
		}
	}
	return nil
}

func regexArguments(arguments map[string]interface{}) error {
	for _, v := range arguments {
		pattern, ok := v.(string)
		if !ok {
			return errors.New("regex arguments are text")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return errors.New("regex")
		}
	}
	return nil
}

type Repository interface {
	List(workspaceID, agentID string) ([]*Scenario, error)
	Find(workspaceID, agentID, scenarioID string) (*Scenario, error)
	Create(s *Scenario) error
	Update(s *Scenario) error
	Delete(workspaceID, agentID, scenarioID string) error
	RecordRun(run *Run) error
	Runs(workspaceID, agentID, scenarioID string, limit int) ([]*Run, error)
	DeleteRunsStartedBefore(before time.Time, limit int) (int64, error)
}
