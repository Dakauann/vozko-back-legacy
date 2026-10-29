package tutorial

import "context"

type Area string

const AreaSites Area = "sites"

type Item struct {
	ID   string
	Done bool
}

type Checklist struct {
	Area  Area
	Items []Item
}

func (c Checklist) DoneCount() int {
	done := 0
	for _, item := range c.Items {
		if item.Done {
			done++
		}
	}
	return done
}

type SiteFacts struct {
	HasPage      bool
	EditedByHand bool
	FormToFunnel bool
	SiteChat     bool
	Booking      bool
	CustomDomain bool
	Published    bool
}

func SitesChecklist(f SiteFacts) Checklist {
	return Checklist{Area: AreaSites, Items: []Item{
		{ID: "create_page", Done: f.HasPage},
		{ID: "edit_by_hand", Done: f.EditedByHand},
		{ID: "form_to_funnel", Done: f.FormToFunnel},
		{ID: "site_chat", Done: f.SiteChat},
		{ID: "booking", Done: f.Booking},
		{ID: "custom_domain", Done: f.CustomDomain},
		{ID: "publish", Done: f.Published},
	}}
}

type SiteFactsReader interface {
	SiteFacts(ctx context.Context, workspaceID string) (SiteFacts, error)
}
