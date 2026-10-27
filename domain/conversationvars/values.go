package conversationvars

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/shared"
)

const (
	MaxNameRunes  = 64
	MaxValueRunes = 2000
)

var (
	ErrNothingToSave = errors.New("conversation variables: send at least one value")
	ErrNameInvalid   = errors.New("conversation variables: a name has only letters, digits and underscores, up to 64")
	ErrNameReserved  = errors.New("conversation variables: lead_name, lead_number and campaign_name are filled by the platform")
	ErrValueInvalid  = errors.New("conversation variables: a value has 1 to 2000 characters")
	ErrNotNeeded     = errors.New("conversation variables: no automation of this workspace needs this variable")
	ErrUnavailable   = errors.New("conversation variables: the conversation values could not be read")
)

var namePattern = regexp.MustCompile(`^\w+$`)

type Lead struct {
	Name   string
	Number string
}

type Campaign struct {
	Name   string
	Values map[string]string
}

type Ref struct {
	WorkspaceID string
	EntryID     string
	EntryType   shared.EntryType
}

type Snapshot struct {
	LeadID string
	Values map[string]string
}

type Stored struct {
	Name  string
	Value string
	SetBy string
	SetAt time.Time
}

type Store interface {
	Values(ctx context.Context, entryID string, entryType shared.EntryType) (map[string]string, error)
	Save(ctx context.Context, ref Ref, values map[string]string, by string) error
}

type CampaignSource interface {
	CampaignOf(ctx context.Context, entryID string, entryType shared.EntryType) (Campaign, error)
}

func Merge(lead Lead, campaign Campaign, stored map[string]string) map[string]string {
	out := make(map[string]string, len(campaign.Values)+len(stored)+3)
	for k, v := range campaign.Values {
		out[k] = v
	}
	for k, v := range stored {
		out[k] = v
	}
	out[automationneeds.KeyLeadName] = lead.Name
	out[automationneeds.KeyLeadNumber] = lead.Number
	out[automationneeds.KeyCampaignName] = campaign.Name
	return out
}

func CampaignValuesOf(metadata map[string]any, variables []string) map[string]string {
	out := agent.MetadataToVars(metadata)
	if out == nil {
		out = make(map[string]string, len(variables))
	}
	for i, v := range variables {
		out[fmt.Sprintf("%d", i+1)] = v
	}
	return out
}

func CampaignVars(values map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
