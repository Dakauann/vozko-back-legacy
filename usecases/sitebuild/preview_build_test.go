package sitebuild

import (
	"context"
	"testing"

	"vozko/domain/site"
)

func TestAPreviewNamesItsBuildSoInspectionsCanBeShared(t *testing.T) {
	f := newFixture(t)
	first, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	if first.Build == "" || first.Build != second.Build {
		t.Fatalf("the same source is the same build: %q %q", first.Build, second.Build)
	}
	other := previewInput()
	other.Project.Files["pages/index.html"] = site.File{Content: "<h1>Novo</h1>\n", Rev: 2}
	changed, err := f.svc.Preview(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Build == first.Build {
		t.Fatal("a change is another build")
	}
}
