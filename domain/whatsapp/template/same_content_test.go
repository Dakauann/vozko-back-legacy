package template

import "testing"

func storedTemplateFixture() *Template {
	url := "https://cdn.example/header.png"
	mediaID := "media-1"
	recommend := true
	minutes := 10
	return &Template{
		ID:              "t1",
		ExternalID:      "ext-1",
		WABAId:          "waba-1",
		Name:            "welcome",
		Language:        "pt_BR",
		Category:        TemplateCategoryUtility,
		Status:          TemplateStatusApproved,
		Quality:         "GREEN",
		ParameterFormat: ParameterFormatPositional,
		HeaderMediaURL:  &url,
		HeaderMediaID:   &mediaID,
		Components: []TemplateComponent{
			{Type: "HEADER", Format: "IMAGE", Example: &TemplateExample{HeaderHandle: []string{"https://scontent.whatsapp.net/a.png?oh=1"}}},
			{Type: "BODY", Text: "Hi {{1}}", Example: &TemplateExample{BodyText: [][]string{{"Ana"}}}},
			{Type: "FOOTER", CodeExpirationMinutes: &minutes, AddSecurityRecommendation: &recommend},
			{Type: "BUTTONS", Buttons: []TemplateButton{{Type: "URL", Text: "Open", URL: "https://x.test/{{1}}", Example: "a"}}},
		},
	}
}

func TestSameContentComparesEveryStoredField(t *testing.T) {
	otherURL := "https://cdn.example/other.png"
	otherMediaID := "media-2"
	emptyMediaID := ""
	otherMinutes := 11
	cases := []struct {
		name   string
		change func(*Template)
		same   bool
	}{
		{name: "identical copy", change: func(*Template) {}, same: true},
		{name: "ids and the account name are not written by an update", change: func(c *Template) { c.ID = "t2"; c.ExternalID = "ext-2"; c.WABAName = "Loja" }, same: true},
		{name: "empty parameter format is stored as positional", change: func(c *Template) { c.ParameterFormat = "" }, same: true},
		{name: "nil and empty slices store the same json", change: func(c *Template) {
			c.Components[1].Parameters = []string{}
			c.Components[0].Buttons = []TemplateButton{}
		}, same: true},
		{name: "waba", change: func(c *Template) { c.WABAId = "waba-2" }},
		{name: "name", change: func(c *Template) { c.Name = "welcome_v2" }},
		{name: "language", change: func(c *Template) { c.Language = "en_US" }},
		{name: "category", change: func(c *Template) { c.Category = TemplateCategoryMarketing }},
		{name: "status", change: func(c *Template) { c.Status = TemplateStatusPaused }},
		{name: "quality", change: func(c *Template) { c.Quality = "YELLOW" }},
		{name: "parameter format", change: func(c *Template) { c.ParameterFormat = ParameterFormatNamed }},
		{name: "header media url", change: func(c *Template) { c.HeaderMediaURL = &otherURL }},
		{name: "header media url cleared", change: func(c *Template) { c.HeaderMediaURL = nil }},
		{name: "header media id", change: func(c *Template) { c.HeaderMediaID = &otherMediaID }},
		{name: "header media id empty is not null", change: func(c *Template) { c.HeaderMediaID = &emptyMediaID }},
		{name: "body text", change: func(c *Template) { c.Components[1].Text = "Hello {{1}}" }},
		{name: "signed header link", change: func(c *Template) {
			c.Components[0].Example = &TemplateExample{HeaderHandle: []string{"https://scontent.whatsapp.net/a.png?oh=2"}}
		}},
		{name: "body example", change: func(c *Template) { c.Components[1].Example = &TemplateExample{BodyText: [][]string{{"Bia"}}} }},
		{name: "code expiration", change: func(c *Template) { c.Components[2].CodeExpirationMinutes = &otherMinutes }},
		{name: "security recommendation cleared", change: func(c *Template) { c.Components[2].AddSecurityRecommendation = nil }},
		{name: "button url", change: func(c *Template) {
			c.Components[3].Buttons = []TemplateButton{{Type: "URL", Text: "Open", URL: "https://y.test/{{1}}", Example: "a"}}
		}},
		{name: "component removed", change: func(c *Template) { c.Components = c.Components[:3] }},
		{name: "components reordered", change: func(c *Template) {
			c.Components[1], c.Components[2] = c.Components[2], c.Components[1]
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored := storedTemplateFixture()
			candidate := storedTemplateFixture()
			tc.change(candidate)
			if got := candidate.SameContent(stored); got != tc.same {
				t.Fatalf("SameContent = %v, want %v", got, tc.same)
			}
			if got := stored.SameContent(candidate); got != tc.same {
				t.Fatalf("SameContent is not symmetric: %v, want %v", got, tc.same)
			}
		})
	}
}

func TestSameContentTreatsNoComponentsAlike(t *testing.T) {
	cases := []struct {
		name  string
		left  []TemplateComponent
		right []TemplateComponent
		same  bool
	}{
		{name: "nil and empty", left: nil, right: []TemplateComponent{}, same: true},
		{name: "nil and one", left: nil, right: []TemplateComponent{{Type: "BODY"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left := &Template{Name: "a", Components: tc.left}
			right := &Template{Name: "a", Components: tc.right}
			if got := left.SameContent(right); got != tc.same {
				t.Fatalf("SameContent = %v, want %v", got, tc.same)
			}
		})
	}
}

func TestSameContentWithNil(t *testing.T) {
	cases := []struct {
		name  string
		left  *Template
		right *Template
		same  bool
	}{
		{name: "both nil", same: true},
		{name: "left nil", right: &Template{}},
		{name: "right nil", left: &Template{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.left.SameContent(tc.right); got != tc.same {
				t.Fatalf("SameContent = %v, want %v", got, tc.same)
			}
		})
	}
}

func TestParameterFormatOrDefault(t *testing.T) {
	cases := []struct {
		in   ParameterFormat
		want ParameterFormat
	}{
		{in: "", want: ParameterFormatPositional},
		{in: ParameterFormatPositional, want: ParameterFormatPositional},
		{in: ParameterFormatNamed, want: ParameterFormatNamed},
	}
	for _, tc := range cases {
		if got := tc.in.OrDefault(); got != tc.want {
			t.Fatalf("%q.OrDefault() = %q, want %q", tc.in, got, tc.want)
		}
	}
}
