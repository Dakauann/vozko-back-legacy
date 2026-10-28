package variabledeclhttp

import (
	"path/filepath"
	"testing"

	"vozko/pkg/codegolden"
)

func TestEveryCodeTheFrontMustTranslateIsListed(t *testing.T) {
	codegolden.Compare(t, filepath.Join("testdata", "error_codes.golden"), Codes(), "update testdata/error_codes.golden and translate conversationVariables.errors.<code> in the pt, en, es and de catalogs")
}
