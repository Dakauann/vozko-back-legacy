package whatsapp_repository

import (
	"testing"

	"vozko/domain/whatsapp/template"
	"vozko/infra/database/schema"
)

func TestAStoredTemplateReadsBackWithTheSameContent(t *testing.T) {
	url := "https://cdn.example/header.png"
	recommend := false
	minutes := 5
	cases := []struct {
		name string
		tmpl *template.Template
	}{
		{name: "no components", tmpl: &template.Template{Name: "empty", Language: "pt_BR"}},
		{name: "empty parameter format", tmpl: &template.Template{Name: "plain", Components: []template.TemplateComponent{{Type: "BODY", Text: "Hi"}}}},
		{name: "every component field", tmpl: &template.Template{
			WABAId: "waba-1", Name: "rich", Language: "pt_BR", Category: template.TemplateCategoryAuthentication, Status: template.TemplateStatusApproved,
			Quality: "GREEN", ParameterFormat: template.ParameterFormatNamed, HeaderMediaURL: &url,
			Components: []template.TemplateComponent{
				{Type: "HEADER", Format: "TEXT", Text: "Hi {{name}}", Example: &template.TemplateExample{HeaderTextNamed: []template.NamedParamExample{{ParamName: "name", Example: "Ana"}}}},
				{Type: "BODY", Text: "Code <b>{{code}}</b> & more", Parameters: []string{}, Example: &template.TemplateExample{BodyText: [][]string{{}, {"x"}}, BodyTextNamed: []template.NamedParamExample{{ParamName: "code", Example: "1"}}}},
				{Type: "FOOTER", AddSecurityRecommendation: &recommend, CodeExpirationMinutes: &minutes},
				{Type: "BUTTONS", Buttons: []template.TemplateButton{
					{Type: "OTP", OTPType: "COPY_CODE"},
					{Type: "FLOW", Text: "Open", FlowID: "f1", FlowAction: "navigate", NavigateScreen: "S1"},
					{Type: "PHONE_NUMBER", Text: "Call", PhoneNumber: "+55"},
				}},
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored := mapToSchema(tc.tmpl)
			value, err := stored.Components.Value()
			if err != nil {
				t.Fatalf("encode components: %v", err)
			}
			var raw []byte
			switch v := value.(type) {
			case []byte:
				raw = v
			case string:
				raw = []byte(v)
			}
			var scanned schema.WhatsAppTemplateComponents
			if err := scanned.Scan(raw); err != nil {
				t.Fatalf("decode components: %v", err)
			}
			stored.Components = scanned
			if read := mapToDomain(stored); !read.SameContent(tc.tmpl) {
				t.Fatalf("read back %+v is not the same content as %+v", read, tc.tmpl)
			}
		})
	}
}
