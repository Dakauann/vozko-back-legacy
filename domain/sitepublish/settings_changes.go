package sitepublish

import (
	"reflect"
	"slices"
	"sort"

	"vozko/domain/site"
)

type ChangeAction string

const (
	ChangeAdded   ChangeAction = "added"
	ChangeRemoved ChangeAction = "removed"
	ChangeChanged ChangeAction = "changed"
)

const (
	FieldFunnel         = "funnel_id"
	FieldStage          = "stage_id"
	FieldOwner          = "owner"
	FieldWorkflow       = "workflow_id"
	FieldFollowUp       = "follow_up"
	FieldSuccess        = "success"
	FieldNotify         = "notify"
	FieldBanner         = "banner"
	FieldText           = "text"
	FieldCategories     = "categories"
	FieldTitle          = "title"
	FieldDescription    = "description"
	FieldSocialImage    = "og_media_id"
	FieldCanonical      = "canonical"
	FieldNoIndex        = "noindex"
	FieldFavicon        = "favicon_media_id"
	FieldStructuredData = "structured_data"
)

type RedirectChange struct {
	Action         ChangeAction `json:"action"`
	From           string       `json:"from"`
	To             string       `json:"to,omitempty"`
	Status         int          `json:"status,omitempty"`
	PreviousTo     string       `json:"previous_to,omitempty"`
	PreviousStatus int          `json:"previous_status,omitempty"`
}

type IntegrationChange struct {
	Action     ChangeAction         `json:"action"`
	Kind       site.IntegrationKind `json:"kind"`
	ID         string               `json:"id,omitempty"`
	PreviousID string               `json:"previous_id,omitempty"`
}

type FormChange struct {
	Action ChangeAction       `json:"action"`
	FormID string             `json:"form_id"`
	Fields []string           `json:"fields"`
	Before *site.FormSettings `json:"before,omitempty"`
	After  *site.FormSettings `json:"after,omitempty"`
}

type ConsentChange struct {
	Fields []string     `json:"fields"`
	Before site.Consent `json:"before"`
	After  site.Consent `json:"after"`
}

type PageChange struct {
	Action ChangeAction   `json:"action"`
	Route  string         `json:"route"`
	Fields []string       `json:"fields"`
	Before *site.PageMeta `json:"before,omitempty"`
	After  *site.PageMeta `json:"after,omitempty"`
}

type IdentityChange struct {
	Fields []string      `json:"fields"`
	Before site.Identity `json:"before"`
	After  site.Identity `json:"after"`
}

type SettingsChanges struct {
	First        bool                `json:"first,omitempty"`
	Redirects    []RedirectChange    `json:"redirects"`
	Integrations []IntegrationChange `json:"integrations"`
	Forms        []FormChange        `json:"forms"`
	Consent      *ConsentChange      `json:"consent,omitempty"`
	Pages        []PageChange        `json:"pages"`
	Site         *IdentityChange     `json:"site,omitempty"`
}

func (c SettingsChanges) Empty() bool {
	return len(c.Redirects) == 0 && len(c.Integrations) == 0 && len(c.Forms) == 0 && c.Consent == nil && len(c.Pages) == 0 && c.Site == nil
}

func (c SettingsChanges) Routing() bool {
	return len(c.Redirects) > 0 || len(c.Integrations) > 0
}

func DiffSettings(published site.Settings, hasPublished bool, current site.Settings) SettingsChanges {
	if !hasPublished {
		published = site.Settings{}
	}
	return SettingsChanges{
		First:        !hasPublished,
		Redirects:    redirectChanges(published.Redirects, current.Redirects),
		Integrations: integrationChanges(published.Integrations, current.Integrations),
		Forms:        formChanges(published.Forms, current.Forms),
		Consent:      consentChange(published.Consent, current.Consent),
		Pages:        pageChanges(published.Pages, current.Pages),
		Site:         identityChange(published.Site, current.Site),
	}
}

func redirectChanges(before, after []site.Redirect) []RedirectChange {
	old := map[string]site.Redirect{}
	for _, r := range before {
		old[r.From] = r
	}
	seen := map[string]bool{}
	out := []RedirectChange{}
	for _, r := range after {
		seen[r.From] = true
		prev, existed := old[r.From]
		switch {
		case !existed:
			out = append(out, RedirectChange{Action: ChangeAdded, From: r.From, To: r.To, Status: r.Status})
		case prev != r:
			out = append(out, RedirectChange{Action: ChangeChanged, From: r.From, To: r.To, Status: r.Status, PreviousTo: prev.To, PreviousStatus: prev.Status})
		}
	}
	for _, r := range before {
		if !seen[r.From] {
			out = append(out, RedirectChange{Action: ChangeRemoved, From: r.From, PreviousTo: r.To, PreviousStatus: r.Status})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

func integrationChanges(before, after site.Integrations) []IntegrationChange {
	out := []IntegrationChange{}
	for _, kind := range site.IntegrationKinds() {
		prev, had := before.ID(kind)
		next, has := after.ID(kind)
		switch {
		case had && !has:
			out = append(out, IntegrationChange{Action: ChangeRemoved, Kind: kind, PreviousID: prev})
		case !had && has:
			out = append(out, IntegrationChange{Action: ChangeAdded, Kind: kind, ID: next})
		case had && prev != next:
			out = append(out, IntegrationChange{Action: ChangeChanged, Kind: kind, ID: next, PreviousID: prev})
		}
	}
	return out
}

func formFields(before, after site.FormSettings) []string {
	var fields []string
	for _, f := range []struct {
		name string
		same bool
	}{
		{FieldFunnel, before.FunnelID == after.FunnelID},
		{FieldStage, before.StageID == after.StageID},
		{FieldOwner, reflect.DeepEqual(before.Owner, after.Owner)},
		{FieldWorkflow, before.WorkflowID == after.WorkflowID},
		{FieldFollowUp, reflect.DeepEqual(before.FollowUp, after.FollowUp)},
		{FieldSuccess, reflect.DeepEqual(before.Success, after.Success)},
		{FieldNotify, before.Notify == after.Notify},
	} {
		if !f.same {
			fields = append(fields, f.name)
		}
	}
	return fields
}

func sortedKeys[V any](maps ...map[string]V) []string {
	var keys []string
	for _, m := range maps {
		for k := range m {
			if !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func actionOf(had, has bool) ChangeAction {
	switch {
	case !had:
		return ChangeAdded
	case !has:
		return ChangeRemoved
	}
	return ChangeChanged
}

func formChanges(before, after map[string]site.FormSettings) []FormChange {
	out := []FormChange{}
	for _, id := range sortedKeys(before, after) {
		prev, had := before[id]
		next, has := after[id]
		fields := formFields(prev, next)
		if len(fields) == 0 && had && has {
			continue
		}
		change := FormChange{Action: actionOf(had, has), FormID: id, Fields: fields}
		if fields == nil {
			change.Fields = []string{}
		}
		if had {
			change.Before = &prev
		}
		if has {
			change.After = &next
		}
		out = append(out, change)
	}
	return out
}

func consentChange(before, after site.Consent) *ConsentChange {
	var fields []string
	if before.Banner != after.Banner {
		fields = append(fields, FieldBanner)
	}
	if before.Text != after.Text {
		fields = append(fields, FieldText)
	}
	if !slices.Equal(before.Categories, after.Categories) {
		fields = append(fields, FieldCategories)
	}
	if len(fields) == 0 {
		return nil
	}
	return &ConsentChange{Fields: fields, Before: before, After: after}
}

func pageFields(before, after site.PageMeta) []string {
	var fields []string
	for _, f := range []struct {
		name string
		same bool
	}{
		{FieldTitle, before.Title == after.Title},
		{FieldDescription, before.Description == after.Description},
		{FieldSocialImage, before.OGMediaID == after.OGMediaID},
		{FieldCanonical, before.Canonical == after.Canonical},
		{FieldNoIndex, before.NoIndex == after.NoIndex},
	} {
		if !f.same {
			fields = append(fields, f.name)
		}
	}
	return fields
}

func pageChanges(before, after map[string]site.PageMeta) []PageChange {
	out := []PageChange{}
	for _, route := range sortedKeys(before, after) {
		prev, had := before[route]
		next, has := after[route]
		fields := pageFields(prev, next)
		if len(fields) == 0 {
			continue
		}
		change := PageChange{Action: actionOf(had, has), Route: route, Fields: fields}
		if had {
			change.Before = &prev
		}
		if has {
			change.After = &next
		}
		out = append(out, change)
	}
	return out
}

func identityChange(before, after site.Identity) *IdentityChange {
	var fields []string
	if before.FaviconMediaID != after.FaviconMediaID {
		fields = append(fields, FieldFavicon)
	}
	if before.StructuredData != after.StructuredData {
		fields = append(fields, FieldStructuredData)
	}
	if len(fields) == 0 {
		return nil
	}
	return &IdentityChange{Fields: fields, Before: before, After: after}
}
