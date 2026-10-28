package automationtransfer

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/stage"
)

const (
	DestinationsKey     = "destinations"
	MaxDestinations     = 10
	MaxDescriptionRunes = 500
	MaxNoteRunes        = 1000
	LoopCap             = 3
	LoopWindow          = 24 * time.Hour
	NoteLifetime        = 72 * time.Hour
)

const (
	keyID           = "id"
	keyKind         = "kind"
	keyAutomationID = "automation_id"
	keyStageID      = "stage_id"
	keyDescription  = "description"
)

var (
	ErrConfigUnreadable     = errors.New("automation transfer: the destinations cannot be read")
	ErrTooManyDestinations  = fmt.Errorf("automation transfer: at most %d destinations", MaxDestinations)
	ErrDescriptionTooLong   = fmt.Errorf("automation transfer: a destination description has at most %d characters", MaxDescriptionRunes)
	ErrNoDestinations       = errors.New("automation transfer: choose at least one destination")
	ErrSelf                 = errors.New("automation transfer: an agent cannot transfer to itself")
	ErrDuplicateDestination = errors.New("automation transfer: two destinations go to the same automation and stage")
	ErrUnknownDestination   = errors.New("automation transfer: the destination is not configured")
	ErrNoteMissing          = errors.New("automation transfer: the hand-off note is missing")
	ErrLoop                 = errors.New("automation transfer: the conversation was transferred too often")
	ErrNotGoverning         = errors.New("automation transfer: the agent does not govern the conversation")
	ErrNoConversation       = errors.New("automation transfer: there is no current conversation")
)

var knownKeys = map[string]bool{keyID: true, keyKind: true, keyAutomationID: true, keyStageID: true, keyDescription: true}

type Destination struct {
	ID          string
	Target      conversation.Automation
	StageID     string
	Description string
	Variables   []VariableSource
}

func Parse(config map[string]interface{}) ([]Destination, error) {
	return parse(config, false)
}

func Normalize(config map[string]interface{}, newID func() string) (map[string]interface{}, error) {
	list, err := parse(config, true)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	items := make([]interface{}, 0, len(list))
	for _, d := range list {
		for d.ID == "" || used[d.ID] {
			d.ID = newID()
		}
		used[d.ID] = true
		items = append(items, d.config())
	}
	return map[string]interface{}{DestinationsKey: items}, nil
}

func (d Destination) config() map[string]interface{} {
	out := map[string]interface{}{keyID: d.ID, keyKind: string(d.Target.Kind), keyAutomationID: d.Target.ID}
	if d.StageID != "" {
		out[keyStageID] = d.StageID
	}
	if d.Description != "" {
		out[keyDescription] = d.Description
	}
	if len(d.Variables) > 0 {
		out[keyVariables] = variablesConfig(d.Variables)
	}
	return out
}

func parse(config map[string]interface{}, idOptional bool) ([]Destination, error) {
	raw, present := config[DestinationsKey]
	if !present || raw == nil {
		return []Destination{}, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, ErrConfigUnreadable
	}
	if len(items) > MaxDestinations {
		return nil, ErrTooManyDestinations
	}
	out := make([]Destination, 0, len(items))
	for i, item := range items {
		d, err := parseOne(item, idOptional)
		if err != nil {
			return nil, At(i+1, err)
		}
		out = append(out, d)
	}
	return out, nil
}

func parseOne(item interface{}, idOptional bool) (Destination, error) {
	fields, ok := item.(map[string]interface{})
	if !ok {
		return Destination{}, ErrConfigUnreadable
	}
	values := make(map[string]string, len(fields))
	var variables []VariableSource
	for key, value := range fields {
		if key == keyVariables {
			parsed, err := parseVariables(value)
			if err != nil {
				return Destination{}, err
			}
			variables = parsed
			continue
		}
		text, isText := value.(string)
		if !knownKeys[key] || !isText {
			return Destination{}, ErrConfigUnreadable
		}
		values[key] = strings.TrimSpace(text)
	}
	target, err := conversation.NewAutomation(values[keyKind], values[keyAutomationID])
	if err != nil {
		return Destination{}, ErrConfigUnreadable
	}
	if values[keyID] == "" && !idOptional {
		return Destination{}, ErrConfigUnreadable
	}
	if len([]rune(values[keyDescription])) > MaxDescriptionRunes {
		return Destination{}, ErrDescriptionTooLong
	}
	return Destination{ID: values[keyID], Target: target, StageID: values[keyStageID], Description: values[keyDescription], Variables: variables}, nil
}

func Check(list []Destination, selfAgentID string) error {
	if len(list) == 0 {
		return ErrNoDestinations
	}
	seen := map[string]bool{}
	for i, d := range list {
		if d.Target.Kind == conversation.AutomationAgent && d.Target.ID == strings.TrimSpace(selfAgentID) {
			return At(i+1, ErrSelf)
		}
		key := string(d.Target.Kind) + "\x00" + d.Target.ID + "\x00" + d.StageID
		if seen[key] {
			return At(i+1, ErrDuplicateDestination)
		}
		seen[key] = true
	}
	return nil
}

func Find(list []Destination, id string) (Destination, bool) {
	id = strings.TrimSpace(id)
	for _, d := range list {
		if id != "" && d.ID == id {
			return d, true
		}
	}
	return Destination{}, false
}

func NoteOf(raw string) (string, error) {
	note := strings.TrimSpace(raw)
	if note == "" {
		return "", ErrNoteMissing
	}
	if runes := []rune(note); len(runes) > MaxNoteRunes {
		note = string(runes[:MaxNoteRunes])
	}
	return note, nil
}

func LoopReached(transfers int64) bool {
	return transfers >= LoopCap
}

var (
	ErrTargetUnusable = errors.New("automation transfer: the destination automation cannot take conversations")
	ErrStageInvalid   = errors.New("automation transfer: the destination stage is not a conversation stage of this workspace")
)

func StageRefused(err error) bool {
	return errors.Is(err, stage.ErrTagNotFound) || errors.Is(err, stage.ErrUnauthorized) || errors.Is(err, stage.ErrStageNotConversationFunnel)
}

type DestinationError struct {
	Position int
	Err      error
}

func At(position int, err error) error {
	return &DestinationError{Position: position, Err: err}
}

func (e *DestinationError) Error() string {
	return fmt.Sprintf("%v (destino %d)", e.Err, e.Position)
}

func (e *DestinationError) Unwrap() error { return e.Err }

func PositionOf(err error) (int, bool) {
	var at *DestinationError
	if errors.As(err, &at) {
		return at.Position, true
	}
	return 0, false
}
