package deps

import (
	"errors"
	"reflect"
	"sort"
	"strings"
)

var ErrMissing = errors.New("missing dependencies")

type MissingError struct {
	Component string
	Missing   []string
}

func (e *MissingError) Error() string {
	return e.Component + ": missing " + strings.Join(e.Missing, ", ")
}

func (e *MissingError) Unwrap() error { return ErrMissing }

func Require(component string, named map[string]any) error {
	var missing []string
	for name, value := range named {
		if IsNil(value) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return &MissingError{Component: component, Missing: missing}
}

func IsNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.UnsafePointer:
		return v.IsNil()
	}
	return false
}
