package agenttool

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	LocationPath   = "path"
	LocationQuery  = "query"
	LocationHeader = "header"
	LocationBody   = "body"

	SourceModel   = "model"
	SourceFixed   = "fixed"
	SourceContext = "context"
)

var ErrHTTPConfigInvalid = errors.New("agenttool: the HTTP tool configuration is not readable")

type HTTPField struct {
	Name        string      `json:"name"`
	Arg         string      `json:"arg,omitempty"`
	Location    string      `json:"location"`
	Source      string      `json:"source"`
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Value       string      `json:"value,omitempty"`
	Properties  []HTTPField `json:"properties,omitempty"`
	Items       *HTTPField  `json:"items,omitempty"`
}

func (f HTTPField) ArgName() string {
	if f.Arg != "" {
		return f.Arg
	}
	return f.Name
}

type HTTPAuth struct {
	Mode    string `json:"mode,omitempty"`
	User    string `json:"user,omitempty"`
	Token   string `json:"token,omitempty"`
	KeyName string `json:"key_name,omitempty"`
}

type HTTPOutputField struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

type HTTPOutput struct {
	Fields        []HTTPOutputField `json:"fields,omitempty"`
	IncludeStatus bool              `json:"include_status,omitempty"`
}

type HTTPConfig struct {
	Method            string      `json:"method"`
	URL               string      `json:"url"`
	Fields            []HTTPField `json:"fields,omitempty"`
	BodyTemplate      string      `json:"body_template,omitempty"`
	Auth              HTTPAuth    `json:"auth,omitempty"`
	Output            HTTPOutput  `json:"output,omitempty"`
	TimeoutSeconds    float64     `json:"timeout_seconds,omitempty"`
	IdempotencyHeader string      `json:"idempotency_header,omitempty"`
}

var canonicalHTTPKeys = map[string]bool{"fields": true, "body_template": true, "auth": true, "output": true, "idempotency_header": true}

var legacyHTTPKeys = map[string]bool{"headers": true, "path_params": true, "query_schema": true, "body_schema": true}

func IsCanonicalHTTPConfig(config map[string]interface{}) bool {
	for key := range config {
		if legacyHTTPKeys[key] {
			return false
		}
	}
	return true
}

func ParseHTTPConfig(config map[string]interface{}) (HTTPConfig, error) {
	plain := map[string]interface{}{}
	for k, v := range config {
		if !strings.HasPrefix(k, "__") {
			plain[k] = v
		}
	}
	raw, err := json.Marshal(plain)
	if err != nil {
		return HTTPConfig{}, fmt.Errorf("%w: %v", ErrHTTPConfigInvalid, err)
	}
	var c HTTPConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return HTTPConfig{}, fmt.Errorf("%w: %v", ErrHTTPConfigInvalid, err)
	}
	if !IsCanonicalHTTPConfig(plain) {
		legacy, err := legacyFields(plain)
		if err != nil {
			return HTTPConfig{}, err
		}
		c.Fields = append(c.Fields, legacy...)
	}
	c.Method = strings.ToUpper(strings.TrimSpace(c.Method))
	if c.Method == "" {
		c.Method = "GET"
	}
	c.URL = strings.TrimSpace(c.URL)
	for i := range c.Fields {
		c.Fields[i] = normalizedField(c.Fields[i])
	}
	disambiguateArgs(c.Fields)
	return c, nil
}

func (c HTTPConfig) Map() map[string]interface{} {
	raw, _ := json.Marshal(c)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func CanonicalHTTPConfig(config map[string]interface{}) (map[string]interface{}, error) {
	parsed, err := ParseHTTPConfig(config)
	if err != nil {
		return nil, err
	}
	out := parsed.Map()
	for k, v := range config {
		if strings.HasPrefix(k, "__") {
			out[k] = v
		}
	}
	return out, nil
}

func (c HTTPConfig) ModelFields() []HTTPField {
	var out []HTTPField
	for _, f := range c.Fields {
		if f.Source == SourceModel {
			out = append(out, f)
		}
	}
	return out
}

func normalizedField(f HTTPField) HTTPField {
	f.Name = strings.TrimSpace(f.Name)
	f.Location = strings.ToLower(strings.TrimSpace(f.Location))
	f.Source = strings.ToLower(strings.TrimSpace(f.Source))
	if f.Source == "" {
		f.Source = SourceModel
	}
	if f.Source == SourceFixed && strings.Contains(f.Value, "{{") {
		f.Source = SourceContext
	}
	f.Type = strings.ToLower(strings.TrimSpace(f.Type))
	if f.Type == "" {
		f.Type = "string"
	}
	return f
}

func disambiguateArgs(fields []HTTPField) {
	count := map[string]int{}
	for _, f := range fields {
		if f.Source == SourceModel {
			count[f.ArgName()]++
		}
	}
	for i, f := range fields {
		if f.Source == SourceModel && f.Arg == "" && count[f.Name] > 1 && f.Location != LocationPath {
			fields[i].Arg = f.Location + "_" + f.Name
		}
	}
}

func legacyFields(config map[string]interface{}) ([]HTTPField, error) {
	var out []HTTPField
	if headers, ok := config["headers"].(map[string]interface{}); ok {
		keys := make([]string, 0, len(headers))
		for k := range headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			value, isText := headers[k].(string)
			if !isText {
				return nil, fmt.Errorf("%w: header %s is not text", ErrHTTPConfigInvalid, k)
			}
			out = append(out, HTTPField{Name: k, Location: LocationHeader, Source: SourceFixed, Value: value})
		}
	}
	for _, group := range []struct{ key, location string }{{"path_params", LocationPath}, {"query_schema", LocationQuery}, {"body_schema", LocationBody}} {
		items, _ := config[group.key].([]interface{})
		for _, item := range items {
			field, ok := legacySchemaField(item)
			if !ok {
				continue
			}
			field.Location = group.location
			out = append(out, field)
		}
	}
	return out, nil
}

func legacySchemaField(item interface{}) (HTTPField, bool) {
	m, ok := item.(map[string]interface{})
	if !ok {
		return HTTPField{}, false
	}
	name, _ := m["name"].(string)
	if strings.TrimSpace(name) == "" {
		return HTTPField{}, false
	}
	f := HTTPField{Name: name, Source: SourceModel}
	f.Type, _ = m["type"].(string)
	f.Description, _ = m["description"].(string)
	f.Required, _ = m["required"].(bool)
	if example, _ := m["example"].(string); example != "" {
		f.Description = strings.TrimSpace(f.Description + " Exemplo: " + example)
	}
	if props, ok := m["properties"].([]interface{}); ok {
		for _, p := range props {
			if child, ok := legacySchemaField(p); ok {
				f.Properties = append(f.Properties, normalizedField(child))
			}
		}
	}
	if items, ok := m["items"].(map[string]interface{}); ok {
		named := map[string]interface{}{"name": "item"}
		for k, v := range items {
			named[k] = v
		}
		if child, ok := legacySchemaField(named); ok {
			child = normalizedField(child)
			f.Items = &child
		}
	}
	return f, true
}
