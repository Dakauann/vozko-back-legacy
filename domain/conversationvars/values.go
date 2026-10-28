package conversationvars

import (
	"context"
	"errors"
	"fmt"
	"time"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/shared"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

const (
	MaxNameRunes  = variabledecl.MaxNameRunes
	MaxValueRunes = variabledecl.MaxValueRunes
)

var (
	ErrNothingToSave = errors.New("conversation variables: send at least one value")
	ErrNameInvalid   = variabledecl.ErrNameInvalid
	ErrNameReserved  = variabledecl.ErrNameReserved
	ErrTypeInvalid   = errors.New("conversation variables: a value does not fit the declared type")
	ErrNotEditable   = errors.New("conversation variables: only declared data or values already stored can be edited")
	ErrValueInvalid  = errors.New("conversation variables: a value has 1 to 2000 characters")
	ErrNotNeeded     = errors.New("conversation variables: no automation of this workspace needs this variable")
	ErrUnavailable   = errors.New("conversation variables: the conversation values could not be read")
)

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

func SeedRun(state *workflow.RunState, values map[string]string) {
	vars := make(map[string]interface{}, len(values))
	for k, v := range values {
		vars[k] = v
	}
	state.Set(workflow.DataKeyCampaignVars, vars)
}
