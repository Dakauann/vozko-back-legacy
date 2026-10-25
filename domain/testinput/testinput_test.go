package testinput

import (
	"errors"
	"strings"
	"testing"

	"vozko/domain/tracepayload"
)

func TestSavedInputsNeedUniqueIDsAndNamesAndFitTheCap(t *testing.T) {
	ok := []TestInput{{ID: "a", Name: "Pedido 1", Input: map[string]interface{}{"x": 1}}, {ID: "b", Name: "Pedido 2"}}
	if err := Validate(ok); err != nil {
		t.Fatal(err)
	}
	cases := [][]TestInput{
		{{ID: "", Name: "x"}},
		{{ID: "a", Name: ""}},
		{{ID: "a", Name: "x"}, {ID: "a", Name: "y"}},
		{{ID: "a", Name: "Igual"}, {ID: "b", Name: "igual"}},
	}
	for _, c := range cases {
		if err := Validate(c); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%+v: %v", c, err)
		}
	}
	big := []TestInput{{ID: "a", Name: "x", Input: map[string]interface{}{"x": strings.Repeat("a", tracepayload.MaxBytes)}}}
	if err := Validate(big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err %v", err)
	}
}
