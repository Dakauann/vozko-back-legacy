package testinput

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"vozko/domain/tracepayload"
)

type TestInput struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Input   map[string]interface{} `json:"input"`
	SavedAt time.Time              `json:"savedAt"`
}

var (
	ErrInvalid  = errors.New("testinput: invalid saved test input")
	ErrTooLarge = fmt.Errorf("%w: input is larger than %d bytes", ErrInvalid, tracepayload.MaxBytes)
)

func Validate(inputs []TestInput) error {
	ids := make(map[string]bool, len(inputs))
	names := make(map[string]bool, len(inputs))
	for _, in := range inputs {
		id, name := strings.TrimSpace(in.ID), strings.ToLower(strings.TrimSpace(in.Name))
		switch {
		case id == "":
			return fmt.Errorf("%w: a test input has no id", ErrInvalid)
		case name == "":
			return fmt.Errorf("%w: a test input has no name", ErrInvalid)
		case ids[id]:
			return fmt.Errorf("%w: the test input id %q repeats", ErrInvalid, id)
		case names[name]:
			return fmt.Errorf("%w: the test input name %q repeats", ErrInvalid, strings.TrimSpace(in.Name))
		}
		ids[id], names[name] = true, true
		encoded, err := json.Marshal(in.Input)
		if err != nil {
			return fmt.Errorf("%w: test input %q is not JSON", ErrInvalid, id)
		}
		if len(encoded) > tracepayload.MaxBytes {
			return fmt.Errorf("%w: test input %q has %d bytes", ErrTooLarge, id, len(encoded))
		}
	}
	return nil
}
