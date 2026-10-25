package agenttool

import (
	"net/url"
	"sort"
	"strings"

	"vozko/domain/redact"
)

const URLParam = "url"

type Credential struct {
	Param string
	Key   string
	Value string
}

func (c Credential) Field() string {
	if c.Key != "" {
		return c.Key
	}
	return c.Param
}

func PlainCredentials(sensitive []string, config map[string]interface{}) []Credential {
	var out []Credential
	for _, param := range sensitive {
		switch value := config[param].(type) {
		case map[string]interface{}:
			for _, key := range sortedKeys(value) {
				if s, ok := value[key].(string); ok && IsCredentialName(key) && isPlain(s) {
					out = append(out, Credential{Param: param, Key: key, Value: s})
				}
			}
		case string:
			if isPlain(value) {
				out = append(out, Credential{Param: param, Value: value})
			}
		}
	}
	if raw, ok := config[URLParam].(string); ok {
		for _, pair := range queryPairs(raw) {
			if IsCredentialName(pair.key) && isPlain(pair.value) {
				out = append(out, Credential{Param: URLParam, Key: pair.key, Value: pair.value})
			}
		}
	}
	return append(out, canonicalCredentials(config)...)
}

func WithReferences(config map[string]interface{}, creds []Credential, names []string) map[string]interface{} {
	out := copyConfig(config)
	for i, c := range creds {
		if i >= len(names) {
			break
		}
		setValue(out, c.Param, c.Key, Reference(names[i]))
	}
	return out
}

func ConcealPlain(sensitive []string, config map[string]interface{}) map[string]interface{} {
	creds := PlainCredentials(sensitive, config)
	if len(creds) == 0 {
		return config
	}
	out := copyConfig(config)
	for _, c := range creds {
		setValue(out, c.Param, c.Key, redact.Mask)
	}
	return out
}

func RestoreMasked(sensitive []string, config, previous map[string]interface{}) map[string]interface{} {
	out := copyConfig(config)
	restoreCanonicalMasks(out, previous)
	for _, param := range sensitive {
		switch value := out[param].(type) {
		case map[string]interface{}:
			prev, _ := previous[param].(map[string]interface{})
			for key, v := range value {
				if v != redact.Mask {
					continue
				}
				if old, ok := prev[key]; ok && old != redact.Mask {
					value[key] = old
				} else {
					delete(value, key)
				}
			}
		case string:
			if value == redact.Mask {
				if old, ok := previous[param]; ok {
					out[param] = old
				} else {
					delete(out, param)
				}
			}
		}
	}
	if raw, ok := out[URLParam].(string); ok {
		prevURL, _ := previous[URLParam].(string)
		prevValues := map[string]string{}
		for _, pair := range queryPairs(prevURL) {
			prevValues[pair.key] = pair.value
		}
		out[URLParam] = rewriteQuery(raw, func(key, value string) (string, bool) {
			if value != redact.Mask {
				return value, true
			}
			old, ok := prevValues[key]
			return old, ok && old != redact.Mask
		})
	}
	return out
}

func isPlain(value string) bool {
	return value != "" && value != redact.Mask && !IsSealed(value)
}

func setValue(config map[string]interface{}, param, key, value string) {
	if param == FieldsParam {
		setFieldValue(config, key, value)
		return
	}
	if param == URLParam && key != "" {
		raw, _ := config[URLParam].(string)
		config[URLParam] = rewriteQuery(raw, func(k, v string) (string, bool) {
			if k == key {
				return value, true
			}
			return v, true
		})
		return
	}
	if key == "" {
		config[param] = value
		return
	}
	entries, _ := config[param].(map[string]interface{})
	copied := make(map[string]interface{}, len(entries))
	for k, v := range entries {
		copied[k] = v
	}
	copied[key] = value
	config[param] = copied
}

type queryPair struct {
	key   string
	value string
}

func splitQuery(raw string) (string, string, string, bool) {
	base, rest, ok := strings.Cut(raw, "?")
	if !ok {
		return raw, "", "", false
	}
	query, fragment, hasFragment := strings.Cut(rest, "#")
	if hasFragment {
		fragment = "#" + fragment
	}
	return base, query, fragment, true
}

func queryPairs(raw string) []queryPair {
	_, query, _, ok := splitQuery(raw)
	if !ok || query == "" {
		return nil
	}
	var out []queryPair
	for _, part := range strings.Split(query, "&") {
		key, value, _ := strings.Cut(part, "=")
		if decoded, err := url.QueryUnescape(key); err == nil {
			key = decoded
		}
		if decoded, err := url.QueryUnescape(value); err == nil && !strings.Contains(value, "{{") {
			value = decoded
		}
		if key != "" {
			out = append(out, queryPair{key: key, value: value})
		}
	}
	return out
}

func rewriteQuery(raw string, rewrite func(key, value string) (string, bool)) string {
	base, query, fragment, ok := splitQuery(raw)
	if !ok || query == "" {
		return raw
	}
	parts := strings.Split(query, "&")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		rawKey, rawValue, hasValue := strings.Cut(part, "=")
		key := rawKey
		if decoded, err := url.QueryUnescape(rawKey); err == nil {
			key = decoded
		}
		value := rawValue
		if decoded, err := url.QueryUnescape(rawValue); err == nil && !strings.Contains(rawValue, "{{") {
			value = decoded
		}
		next, keep := rewrite(key, value)
		if !keep {
			continue
		}
		if next == value || !hasValue && next == "" {
			kept = append(kept, part)
			continue
		}
		if strings.Contains(next, "{{") || next == redact.Mask {
			kept = append(kept, rawKey+"="+next)
			continue
		}
		kept = append(kept, rawKey+"="+url.QueryEscape(next))
	}
	return base + "?" + strings.Join(kept, "&") + fragment
}

func copyConfig(config map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(config))
	for k, v := range config {
		if m, ok := v.(map[string]interface{}); ok {
			copied := make(map[string]interface{}, len(m))
			for mk, mv := range m {
				copied[mk] = mv
			}
			out[k] = copied
			continue
		}
		out[k] = v
	}
	return out
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func ValueAt(config map[string]interface{}, param, key string) string {
	if param == FieldsParam {
		value, _ := fieldValue(config, key)
		return value
	}
	if param == URLParam && key != "" {
		raw, _ := config[URLParam].(string)
		for _, pair := range queryPairs(raw) {
			if pair.key == key {
				return pair.value
			}
		}
		return ""
	}
	if key == "" {
		s, _ := config[param].(string)
		return s
	}
	entries, _ := config[param].(map[string]interface{})
	s, _ := entries[key].(string)
	return s
}

const (
	FieldsParam  = "fields"
	AuthParam    = "auth"
	AuthTokenKey = "token"
)

func canonicalCredentials(config map[string]interface{}) []Credential {
	var out []Credential
	for _, f := range fieldMaps(config) {
		name, _ := f["name"].(string)
		location, _ := f["location"].(string)
		source, _ := f["source"].(string)
		value, _ := f["value"].(string)
		if source == "model" || (location != "header" && location != "query") {
			continue
		}
		if IsCredentialName(name) && isPlain(value) {
			out = append(out, Credential{Param: FieldsParam, Key: name, Value: value})
		}
	}
	if auth, ok := config[AuthParam].(map[string]interface{}); ok {
		mode, _ := auth["mode"].(string)
		token, _ := auth[AuthTokenKey].(string)
		if mode != "" && isPlain(token) {
			out = append(out, Credential{Param: AuthParam, Key: AuthTokenKey, Value: token})
		}
	}
	return out
}

func fieldMaps(config map[string]interface{}) []map[string]interface{} {
	raw, _ := config[FieldsParam].([]interface{})
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func fieldValue(config map[string]interface{}, name string) (string, bool) {
	for _, f := range fieldMaps(config) {
		if f["name"] == name {
			value, _ := f["value"].(string)
			return value, true
		}
	}
	return "", false
}

func setFieldValue(config map[string]interface{}, name, value string) {
	raw, _ := config[FieldsParam].([]interface{})
	copied := make([]interface{}, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			copied[i] = item
			continue
		}
		field := make(map[string]interface{}, len(m))
		for k, v := range m {
			field[k] = v
		}
		if field["name"] == name {
			field["value"] = value
		}
		copied[i] = field
	}
	config[FieldsParam] = copied
}

func restoreCanonicalMasks(config, previous map[string]interface{}) {
	for _, f := range fieldMaps(config) {
		name, _ := f["name"].(string)
		if f["value"] != redact.Mask {
			continue
		}
		old, ok := fieldValue(previous, name)
		if !ok || old == redact.Mask {
			old = ""
		}
		setFieldValue(config, name, old)
	}
	if auth, ok := config[AuthParam].(map[string]interface{}); ok && auth[AuthTokenKey] == redact.Mask {
		old := ValueAt(previous, AuthParam, AuthTokenKey)
		if old == redact.Mask {
			old = ""
		}
		setValue(config, AuthParam, AuthTokenKey, old)
	}
}
