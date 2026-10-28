package site

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"vozko/domain/sitehtml"
)

func TestErrorCodeMapsEverySentinel(t *testing.T) {
	cases := map[error]string{
		ErrStackInvalid:        "stack_invalid",
		ErrPathInvalid:         "path_invalid",
		ErrManifestInvalid:     "site_json_invalid",
		ErrKnobsInvalid:        "knobs_invalid",
		ErrKnobValueInvalid:    "knob_value_invalid",
		ErrSettingsInvalid:     "settings_invalid",
		ErrFormChannelRequired: "form_channel_required",
		ErrDesignSystemInvalid: "design_system_invalid",
		ErrRedirectInvalid:     "redirect_invalid",
		ErrStepsInvalid:        "steps_invalid",
		ErrBatchMalformed:      "batch_malformed",
		ErrEditRefused:         "edit_refused",
		sitehtml.ErrMalformed:  "html_invalid",
	}
	for err, code := range cases {
		if got := ErrorCode(fmt.Errorf("wrapped: %w", err)); got != code {
			t.Fatalf("%v: got %q", err, got)
		}
	}
	if ErrorCode(ValidatePath("vue", "site.json")) != "stack_invalid" {
		t.Fatal("an unknown stack reads as stack_invalid")
	}
	if ErrorCode(&BatchError{}) != "edit_refused" || ErrorCode(nil) != "" || ErrorCode(errors.New("x")) != "" {
		t.Fatal("fallbacks")
	}
}

func TestCodesAreUniqueSnakeCaseAndCoverTheGaps(t *testing.T) {
	snake := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	seen := map[string]bool{}
	for _, code := range Codes() {
		if !snake.MatchString(code) || seen[code] {
			t.Fatalf("%q", code)
		}
		seen[code] = true
	}
	for _, code := range append(GapCodes(), "node_stale", "edit_not_found", "edit_ambiguous", "edit_placeholder", "css_directive_forbidden", "credential_field_forbidden", "frame_source_forbidden", "form_identity_missing", "form_field_unknown", "consent_required", "path_invalid", "file_too_large", "project_too_large", "script_source_forbidden", "file_stale", "knob_exists", "whatsapp_text_too_long", "whatsapp_number_unknown", "import_dynamic_forbidden", "operation_failed") {
		if !seen[code] {
			t.Fatalf("%s is missing", code)
		}
	}
}
