package deps

import (
	"errors"
	"reflect"
	"testing"
)

type port interface{ Do() }

type impl struct{}

func (*impl) Do() {}

func TestRequirePassesWhenEveryDependencyIsPresent(t *testing.T) {
	var p port = &impl{}
	err := Require("billing", map[string]any{
		"port":  p,
		"func":  func() {},
		"map":   map[string]int{},
		"value": 3,
	})
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
}

func TestRequireNamesEveryMissingDependencySorted(t *testing.T) {
	err := Require("billing", map[string]any{
		"ledger": nil,
		"clock":  nil,
		"pricer": &impl{},
	})
	var missing *MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("want a MissingError, got %v", err)
	}
	if missing.Component != "billing" {
		t.Fatalf("component = %q", missing.Component)
	}
	if !reflect.DeepEqual(missing.Missing, []string{"clock", "ledger"}) {
		t.Fatalf("missing = %v", missing.Missing)
	}
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("want errors.Is(err, ErrMissing)")
	}
	if got := err.Error(); got != "billing: missing clock, ledger" {
		t.Fatalf("message = %q", got)
	}
}

func TestRequireTreatsTypedNilsAsMissing(t *testing.T) {
	var nilImpl *impl
	var nilPort port = nilImpl
	var nilFunc func()
	var nilMap map[string]int
	var nilSlice []int
	var nilChan chan int

	cases := map[string]any{
		"pointer in interface": nilPort,
		"func":                 nilFunc,
		"map":                  nilMap,
		"slice":                nilSlice,
		"chan":                 nilChan,
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Require("c", map[string]any{name: value}); !errors.Is(err, ErrMissing) {
				t.Fatalf("want missing for %s, got %v", name, err)
			}
		})
	}
}

func TestIsNilSeesThroughInterfaces(t *testing.T) {
	var nilImpl *impl
	if !IsNil(nil) || !IsNil(nilImpl) || !IsNil(port(nilImpl)) {
		t.Fatal("want nil and typed nils reported as nil")
	}
	if IsNil(&impl{}) || IsNil(0) || IsNil("") || IsNil(struct{}{}) {
		t.Fatal("want present values reported as not nil")
	}
}
