package workspace

import "testing"

func TestSitesResourcesCarryTheirActionsAndRisks(t *testing.T) {
	want := map[Resource][]Action{
		ResourceSites:           {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionPublish},
		ResourceSiteDomains:     {ActionRead, ActionUpdate},
		ResourceSiteSubmissions: {ActionRead},
	}
	names := map[Resource]string{ResourceSites: "sites", ResourceSiteDomains: "site_domains", ResourceSiteSubmissions: "site_submissions"}
	for resource, actions := range want {
		if string(resource) != names[resource] || !resource.IsValid() {
			t.Fatalf("resource %q", resource)
		}
		defs := map[Action]ActionDefinition{}
		for _, def := range ResourceActions[resource] {
			if def.Description == "" {
				t.Errorf("%s:%s has no description", resource, def.ActionName)
			}
			defs[def.ActionName] = def
		}
		if len(defs) != len(actions) {
			t.Errorf("%s has %d actions, want %d", resource, len(defs), len(actions))
		}
		for _, action := range actions {
			if _, ok := defs[action]; !ok {
				t.Errorf("%s misses %s", resource, action)
			}
		}
	}
	if ActionPublish != "publish" || !ActionPublish.IsValid() {
		t.Fatalf("publish = %q", ActionPublish)
	}
	if !hasRisk(actionRisks(ResourceSites, ActionPublish), RiskContactsCustomers) {
		t.Error("publishing puts a page in front of customers and must say so")
	}
	if !hasRisk(actionRisks(ResourceSites, ActionDelete), RiskDeletesData) {
		t.Error("deleting pages deletes data and must say so")
	}
	if !hasRisk(actionRisks(ResourceSiteSubmissions, ActionRead), RiskSensitiveData) {
		t.Error("submissions are personal data and must say so")
	}
}

func TestTheSitesFeatureOpensThePagesScreens(t *testing.T) {
	feature, ok := FeatureByKey("sites")
	if !ok {
		t.Fatal("no sites feature")
	}
	keys := map[CapabilityKey]bool{}
	for _, c := range feature.Capabilities {
		keys[c.Key] = true
	}
	for _, key := range []CapabilityKey{"sites.view", "sites.create", "sites.edit", "sites.delete", "sites.publish", "sites.domains", "sites.submissions"} {
		if !keys[key] {
			t.Errorf("missing capability %s", key)
		}
	}
	for screen, want := range map[Screen]CapabilityKey{ScreenSites: "sites.view", ScreenSiteEditor: "sites.view"} {
		use, ok := CapabilityForScreen(screen)
		if !ok || use.Capability.Key != want {
			t.Errorf("screen %s opened by %v, want %s", screen, use.Capability.Key, want)
		}
	}
	if params := screenParams[ScreenSiteEditor]; len(params) != 1 || params[0] != "projectId" {
		t.Errorf("editor params = %v", params)
	}
	for _, s := range feature.Scopes {
		if s == ScopeDepartments {
			t.Error("pages have no department scope (plan D15)")
		}
	}
}

func actionRisks(resource Resource, action Action) []RiskKind {
	for _, def := range ResourceActions[resource] {
		if def.ActionName == action {
			return def.Risks
		}
	}
	return nil
}

func TestPresetsThatBuildCampaignsAlsoBuildPages(t *testing.T) {
	for key, want := range map[RolePresetKey][]CapabilityKey{
		PresetMarketing: {"sites.view", "sites.create", "sites.edit", "sites.publish", "sites.submissions"},
		PresetManager:   {"sites.view", "sites.create", "sites.edit", "sites.publish", "sites.submissions"},
		PresetAnalyst:   {"sites.view"},
	} {
		preset, _ := RolePresetByKey(key)
		held := map[CapabilityKey]bool{}
		for _, c := range preset.Capabilities {
			held[c] = true
		}
		for _, c := range want {
			if !held[c] {
				t.Errorf("%s misses %s", key, c)
			}
		}
	}
}
