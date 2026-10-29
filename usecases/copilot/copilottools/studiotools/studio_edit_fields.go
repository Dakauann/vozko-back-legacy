package studiotools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/shared"
	"vozko/usecases/copilot/copilottools"
)

const studioNearFieldDistance = 2

type studioUnknownField struct {
	path   string
	top    string
	nearby string
}

func (t *studioEditTool) opType() reflect.Type {
	if t.studioScope.kind == copilot.StudioImage {
		return reflect.TypeOf(studioImageOperation{})
	}
	return reflect.TypeOf(studioVideoOperation{})
}

func (t *studioEditTool) refuseUnknownFields(args map[string]interface{}) error {
	raw, err := json.Marshal(args["operations"])
	if err != nil {
		return nil
	}
	var ops []interface{}
	if json.Unmarshal(raw, &ops) != nil {
		return nil
	}
	names := t.names()
	var issues []string
	for i, op := range ops {
		var found []studioUnknownField
		collectUnknownFields(op, t.opType(), "", "", &found)
		for _, field := range found {
			issues = append(issues, fmt.Sprintf("operação %d: o campo %q não existe em %s%s; %s: confira os campos com studio_op_guide", i+1, field.path, t.name, field.hint(), names.nearestFamily(field.familyKey())))
		}
	}
	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", copilottools.ErrInvalidArgs, strings.Join(issues, "; "))
}

func (f studioUnknownField) hint() string {
	if f.nearby == "" {
		return ""
	}
	return fmt.Sprintf(" (você quis dizer %q?)", f.nearby)
}

func (f studioUnknownField) familyKey() string {
	switch {
	case f.top != "":
		return f.top
	case f.nearby != "":
		return f.nearby
	}
	return f.path
}

func collectUnknownFields(value interface{}, t reflect.Type, path, top string, found *[]studioUnknownField) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		fields, ok := value.(map[string]interface{})
		if !ok {
			return
		}
		known := jsonFieldTypes(t)
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := key
			if path != "" {
				child = path + "." + key
			}
			fieldType, ok := known[key]
			if !ok {
				if !slices.ContainsFunc(*found, func(f studioUnknownField) bool { return f.path == child }) {
					*found = append(*found, studioUnknownField{path: child, top: top, nearby: nearestName(key, known)})
				}
				continue
			}
			owner := top
			if owner == "" {
				owner = key
			}
			collectUnknownFields(fields[key], fieldType, child, owner, found)
		}
	case reflect.Slice:
		items, ok := value.([]interface{})
		if !ok {
			return
		}
		for _, item := range items {
			collectUnknownFields(item, t.Elem(), path, top, found)
		}
	}
}

func jsonFieldTypes(t reflect.Type) map[string]reflect.Type {
	known := map[string]reflect.Type{}
	for _, f := range reflect.VisibleFields(t) {
		if name := copilottools.JSONName(f); name != "" {
			known[name] = f.Type
		}
	}
	return known
}

func nearestName(key string, known map[string]reflect.Type) string {
	best, bestScore := "", min(studioNearFieldDistance, len(key)/2)+1
	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if score := shared.EditDistance(key, name); score < bestScore {
			best, bestScore = name, score
		}
	}
	return best
}
