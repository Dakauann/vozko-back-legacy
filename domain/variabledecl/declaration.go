package variabledecl

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	KeyLeadName     = "lead_name"
	KeyLeadNumber   = "lead_number"
	KeyCampaignName = "campaign_name"

	MaxNameRunes        = 64
	MaxLabelRunes       = 60
	MaxDescriptionRunes = 500
	MaxValueRunes       = 2000
)

var (
	ErrNameInvalid        = errors.New("variable declaration: a name has only letters, digits and underscores, up to 64")
	ErrNameReserved       = errors.New("variable declaration: lead_name, lead_number and campaign_name are filled by the platform")
	ErrLabelInvalid       = errors.New("variable declaration: a friendly name has 1 to 60 characters")
	ErrDescriptionTooLong = errors.New("variable declaration: a description has up to 500 characters")
	ErrKindUnknown        = errors.New("variable declaration: unknown type")
	ErrTypeInvalid        = errors.New("variable declaration: the type settings are invalid")
	ErrValueInvalid       = errors.New("variable declaration: the value does not fit the declared type")
	ErrExampleInvalid     = errors.New("variable declaration: the example does not fit the declared type")
	ErrKindTaken          = errors.New("variable declaration: two rules for one type")
	ErrNotFound           = errors.New("variable declaration: no such declaration in this workspace")
	ErrUnreadable         = errors.New("variable declaration: the declarations could not be read")
)

var namePattern = regexp.MustCompile(`^\w{1,64}$`)

var systemKeys = []string{KeyLeadName, KeyLeadNumber, KeyCampaignName}

var systemExamples = map[string]string{
	KeyLeadName:     "Ana Souza",
	KeyLeadNumber:   "5511999990000",
	KeyCampaignName: "Matrículas 2026",
}

func SystemExample(name string) string {
	return systemExamples[name]
}

var systemLabels = map[string]string{
	KeyLeadName:     "Nome do contato",
	KeyLeadNumber:   "Número do contato",
	KeyCampaignName: "Nome da campanha",
}

func SystemLabel(name string) (string, bool) {
	label, ok := systemLabels[name]
	return label, ok
}

func SystemLabelKey(name string) string {
	return "conversation_" + name
}

func SystemKeys() []string {
	return append([]string(nil), systemKeys...)
}

func IsSystemKey(name string) bool {
	for _, key := range systemKeys {
		if key == name {
			return true
		}
	}
	return false
}

type Kind string

type Shape string

const (
	ShapeText   Shape = "text"
	ShapeNumber Shape = "number"
	ShapeDate   Shape = "date"
	ShapeChoice Shape = "choice"
)

type Type struct {
	Kind     Kind              `json:"kind"`
	Options  []string          `json:"options,omitempty"`
	Settings map[string]string `json:"settings,omitempty"`
}

type Declaration struct {
	Name        string
	Label       string
	Description string
	Example     string
	Type        Type
	UpdatedBy   string
	UpdatedAt   time.Time
}

type Check struct {
	WorkspaceID string
	Name        string
	Type        Type
	Value       string
}

type Rule interface {
	Kind() Kind
	Shape() Shape
	Sample(t Type) string
	CheckType(t Type) error
	Accepts(ctx context.Context, c Check) error
}

type Repository interface {
	List(ctx context.Context, workspaceID string) ([]Declaration, error)
	Save(ctx context.Context, workspaceID string, d Declaration) error
	Delete(ctx context.Context, workspaceID, name string) (bool, error)
}

func CheckName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrNameInvalid, name)
	}
	if IsSystemKey(name) {
		return fmt.Errorf("%w: %q", ErrNameReserved, name)
	}
	return nil
}

func normalized(d Declaration) Declaration {
	d.Name = strings.TrimSpace(d.Name)
	d.Label = strings.TrimSpace(d.Label)
	d.Description = strings.TrimSpace(d.Description)
	d.Example = strings.TrimSpace(d.Example)
	d.Type.Kind = Kind(strings.TrimSpace(string(d.Type.Kind)))
	if len(d.Type.Options) > 0 {
		options := make([]string, len(d.Type.Options))
		for i, option := range d.Type.Options {
			options[i] = strings.TrimSpace(option)
		}
		d.Type.Options = options
	}
	if len(d.Type.Settings) == 0 {
		d.Type.Settings = nil
	}
	return d
}

func checkPresentation(d Declaration) error {
	if err := CheckName(d.Name); err != nil {
		return err
	}
	if n := utf8.RuneCountInString(d.Label); n == 0 || n > MaxLabelRunes {
		return ErrLabelInvalid
	}
	if utf8.RuneCountInString(d.Description) > MaxDescriptionRunes {
		return ErrDescriptionTooLong
	}
	return nil
}
