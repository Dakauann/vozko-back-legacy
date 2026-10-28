package sitepublish_repository

import (
	"context"
	"testing"
	"time"
)

func TestServingKeysPutReplaceGetAndDelete(t *testing.T) {
	db := postgresDB(t)
	keys := NewServingKeys(db, func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	if _, found, err := keys.Get(ctx, "host:sorriso.paginas.test"); err != nil || found {
		t.Fatalf("got %v %v", found, err)
	}
	if err := keys.Put(ctx, "host:sorriso.paginas.test", []byte(`{"v":1,"deployment":"d1"}`)); err != nil {
		t.Fatal(err)
	}
	if err := keys.Put(ctx, "host:sorriso.paginas.test", []byte(`{"v":1,"deployment":"d2"}`)); err != nil {
		t.Fatal(err)
	}
	value, found, err := keys.Get(ctx, "host:sorriso.paginas.test")
	if err != nil || !found || string(value) != `{"v":1,"deployment":"d2"}` {
		t.Fatalf("got %s %v %v", value, found, err)
	}
	if err := keys.Delete(ctx, "host:sorriso.paginas.test"); err != nil {
		t.Fatal(err)
	}
	if err := keys.Delete(ctx, "host:sorriso.paginas.test"); err != nil {
		t.Fatal("deleting a missing key is fine")
	}
	if _, found, _ := keys.Get(ctx, "host:sorriso.paginas.test"); found {
		t.Fatal("the key is gone")
	}
}
