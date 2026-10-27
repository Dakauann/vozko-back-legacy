package personalinboxhttp

import "vozko/domain/personalinbox"

type InboxPreferencesRequest struct {
	HideGroups *bool `json:"hideGroups"`
}

type InboxPreferencesResponse struct {
	HideGroups bool `json:"hideGroups"`
}

func presentPreferences(prefs personalinbox.Preferences) InboxPreferencesResponse {
	return InboxPreferencesResponse{HideGroups: prefs.HideGroups}
}
