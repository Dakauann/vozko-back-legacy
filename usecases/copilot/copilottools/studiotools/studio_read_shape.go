package studiotools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	studioMaxCheckLines   = 12
	studioReadWholeChars  = 12000
	studioOutlineTextRune = 80
	studioReadOutlineNote = "projeto grande: isto é o esboço (ids, tipos, tempos, caixas e textos curtos). Antes de editar estilo, animação, chaves ou efeitos, leia por inteiro só o que vai mexer, com ids, track_id ou artboard_id; changes traz só o que o usuário mudou"
	studioReadUnchanged   = "nada mudou desde a sua última leitura"
	studioReadNoTrack     = "a faixa %s não existe no projeto; confira os ids com studio_read"
	studioReadNoArtboard  = "a prancheta %s não existe no projeto; confira os ids com studio_read"
	studioReadNoItems     = "nenhum destes ids existe no projeto: %s; confira os ids com studio_read"
)

var (
	studioFrameAlert       = regexp.MustCompile(`^(erro|aviso): em ([0-9.,]+) s: (.+)$`)
	studioOutlineKeys      = []string{"id", "index", "z", "type", "kind", "name", "text", "shape", "media", "media_id", "start_ms", "duration_ms", "end_ms", "box", "group", "parent", "linked", "component", "hidden", "locked", "muted", "disabled", "animated"}
	studioOutlineTrackKeys = []string{"id", "index", "kind", "name", "hidden", "locked", "muted"}
	studioOutlineBoardKeys = []string{"id", "name", "width", "height", "x", "y", "background", "component", "instances", "alerts"}
	studioOutlineGroupKeys = []string{"id", "parent", "name", "base"}
	studioOutlineEdgeKeys  = []string{"id", "from", "to", "kind", "window_ms"}
	studioRangedDropped    = []string{"tracks", "artboards", "markers", "library", "icons", "styles"}
)

type studioReadScope struct {
	ids        []string
	trackID    string
	artboardID string
	changes    bool
}

func decodeStudioRead(raw []byte) (map[string]interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc map[string]interface{}
	if err := dec.Decode(&doc); err != nil || doc == nil {
		return nil, fmt.Errorf("%s", studioEditorGarbled)
	}
	return doc, nil
}

func shapeStudioRead(raw []byte, scope studioReadScope) (map[string]interface{}, error) {
	doc, err := decodeStudioRead(raw)
	if err != nil {
		return nil, err
	}
	shapeAlerts(doc)
	switch {
	case len(scope.ids) > 0:
		return readItems(doc, scope.ids, true)
	case scope.trackID != "":
		return readContainer(doc, "tracks", "track", scope.trackID, studioReadNoTrack)
	case scope.artboardID != "":
		return readContainer(doc, "artboards", "artboard", scope.artboardID, studioReadNoArtboard)
	case scope.changes:
		return readChanges(doc)
	}
	if len(raw) <= studioReadWholeChars {
		return doc, nil
	}
	return outline(doc), nil
}

func shapeAlerts(doc map[string]interface{}) {
	if alerts, ok := doc["alerts"].([]interface{}); ok {
		doc["alerts"] = cappedAlerts(alerts)
	}
	boards, _ := doc["artboards"].([]interface{})
	for _, board := range boards {
		if fields, ok := board.(map[string]interface{}); ok {
			if alerts, ok := fields["alerts"].([]interface{}); ok {
				fields["alerts"] = cappedAlerts(alerts)
			}
		}
	}
}

type mergedAlert struct {
	level string
	text  string
	times []string
}

func cappedAlerts(alerts []interface{}) []interface{} {
	var merged []*mergedAlert
	index := map[string]*mergedAlert{}
	for _, alert := range alerts {
		line, ok := alert.(string)
		if !ok {
			continue
		}
		level, at, text := "", "", line
		if m := studioFrameAlert.FindStringSubmatch(line); m != nil {
			level, at, text = m[1], m[2], m[3]
		}
		key := level + "\x00" + text
		entry, seen := index[key]
		if !seen {
			entry = &mergedAlert{level: level, text: text}
			index[key] = entry
			merged = append(merged, entry)
		}
		if at != "" && !slices.Contains(entry.times, at) {
			entry.times = append(entry.times, at)
		}
	}
	lines := make([]interface{}, 0, min(len(merged), studioMaxCheckLines)+1)
	for _, entry := range merged[:min(len(merged), studioMaxCheckLines)] {
		lines = append(lines, entry.line())
	}
	if len(merged) > studioMaxCheckLines {
		lines = append(lines, fmt.Sprintf("e mais %d pontos parecidos", len(merged)-studioMaxCheckLines))
	}
	return lines
}

func (a *mergedAlert) line() string {
	if len(a.times) == 0 {
		return a.text
	}
	times := a.times[0]
	if n := len(a.times); n > 1 {
		times = strings.Join(a.times[:n-1], ", ") + " e " + a.times[n-1]
	}
	return a.level + ": em " + times + " s: " + a.text
}

func projectFacts(doc map[string]interface{}) map[string]interface{} {
	facts := make(map[string]interface{}, len(doc))
	for key, value := range doc {
		if !slices.Contains(studioRangedDropped, key) {
			facts[key] = value
		}
	}
	return facts
}

func readContainer(doc map[string]interface{}, list, key, id, missing string) (map[string]interface{}, error) {
	items, _ := doc[list].([]interface{})
	for _, item := range items {
		if fields, ok := item.(map[string]interface{}); ok && fields["id"] == id {
			facts := projectFacts(doc)
			facts[key] = fields
			return facts, nil
		}
	}
	return nil, fmt.Errorf(missing, id)
}

func readItems(doc map[string]interface{}, ids []string, strict bool) (map[string]interface{}, error) {
	var found []interface{}
	seen := map[string]bool{}
	for _, key := range []string{"tracks", "artboards", "markers"} {
		findItems(doc[key], ids, "", seen, &found)
	}
	var missing []string
	for _, id := range ids {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	if strict && len(found) == 0 {
		return nil, fmt.Errorf(studioReadNoItems, strings.Join(ids, ", "))
	}
	facts := projectFacts(doc)
	facts["items"] = found
	if len(missing) > 0 {
		facts["missing"] = missing
	}
	return facts, nil
}

func findItems(value interface{}, ids []string, in string, seen map[string]bool, found *[]interface{}) {
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {
			findItems(item, ids, in, seen, found)
		}
	case map[string]interface{}:
		id, _ := v["id"].(string)
		if id != "" && slices.Contains(ids, id) && !seen[id] {
			seen[id] = true
			item := make(map[string]interface{}, len(v)+1)
			for key, field := range v {
				item[key] = field
			}
			if in != "" {
				item["in"] = in
			}
			*found = append(*found, item)
		}
		inner := in
		if id != "" {
			inner = id
		}
		for _, field := range v {
			findItems(field, ids, inner, seen, found)
		}
	}
}

func readChanges(doc map[string]interface{}) (map[string]interface{}, error) {
	changes, ok := doc["changes_by_user"].(map[string]interface{})
	if !ok {
		facts := projectFacts(doc)
		facts["read"] = studioReadUnchanged
		return facts, nil
	}
	var ids []string
	for _, key := range []string{"added", "changed"} {
		list, _ := changes[key].([]interface{})
		for _, id := range list {
			if text, ok := id.(string); ok {
				ids = append(ids, text)
			}
		}
	}
	return readItems(doc, ids, false)
}

func outline(doc map[string]interface{}) map[string]interface{} {
	shaped := make(map[string]interface{}, len(doc)+1)
	for key, value := range doc {
		shaped[key] = value
	}
	if tracks, ok := doc["tracks"].([]interface{}); ok {
		shaped["tracks"] = outlineTracks(tracks)
	}
	if boards, ok := doc["artboards"].([]interface{}); ok {
		shaped["artboards"] = outlineList(boards, func(board map[string]interface{}) map[string]interface{} {
			out := pick(board, studioOutlineBoardKeys)
			if layers, ok := board["layers"].([]interface{}); ok {
				out["layers"] = outlineList(layers, outlineItem)
			}
			if groups, ok := board["groups"].([]interface{}); ok && len(groups) > 0 {
				out["groups"] = outlineList(groups, func(group map[string]interface{}) map[string]interface{} { return pick(group, studioOutlineGroupKeys) })
			}
			return out
		})
	}
	shaped["read"] = studioReadOutlineNote
	return shaped
}

func outlineTracks(tracks []interface{}) []interface{} {
	return outlineList(tracks, func(track map[string]interface{}) map[string]interface{} {
		out := pick(track, studioOutlineTrackKeys)
		if clips, ok := track["clips"].([]interface{}); ok {
			out["clips"] = outlineList(clips, outlineItem)
		}
		if edges, ok := track["transitions"].([]interface{}); ok {
			out["transitions"] = outlineList(edges, func(edge map[string]interface{}) map[string]interface{} { return pick(edge, studioOutlineEdgeKeys) })
		}
		return out
	})
}

func outlineItem(item map[string]interface{}) map[string]interface{} {
	out := pick(item, studioOutlineKeys)
	if text, ok := out["text"].(string); ok {
		if runes := []rune(text); len(runes) > studioOutlineTextRune {
			out["text"] = string(runes[:studioOutlineTextRune]) + "…"
		}
	}
	if group, ok := out["group"].(map[string]interface{}); ok {
		nested := pick(group, []string{"name", "content_ms"})
		if tracks, ok := group["tracks"].([]interface{}); ok {
			nested["tracks"] = outlineTracks(tracks)
		}
		out["group"] = nested
	}
	return out
}

func outlineList(items []interface{}, shape func(map[string]interface{}) map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		if fields, ok := item.(map[string]interface{}); ok {
			out = append(out, shape(fields))
			continue
		}
		out = append(out, item)
	}
	return out
}

func pick(fields map[string]interface{}, keys []string) map[string]interface{} {
	out := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			out[key] = value
		}
	}
	return out
}
