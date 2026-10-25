package agenttool

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"vozko/domain/calltrace"
	"vozko/domain/tools"
)

const (
	ScopeArgs         = "args"
	ScopeVar          = "var"
	ScopeConversation = "conversation"
	ScopeLead         = "lead"
	ScopeSecret       = "secret"
)

var (
	templatePattern   = regexp.MustCompile(`\{\{\s*([a-z]+)\.([A-Za-z0-9_]+)\s*\}\}`)
	outputPathPattern = regexp.MustCompile(`^[A-Za-z0-9_\-]+(\[[0-9]+\])*(\.[A-Za-z0-9_\-]+(\[[0-9]+\])*)*$`)
	pathSegment       = regexp.MustCompile(`([^.\[\]]+)|\[([0-9]+)\]`)
)

type Lookup func(scope, key string) (string, bool)

type CallError struct {
	Kind    calltrace.ErrorKind
	Message string
}

func (e *CallError) Error() string { return e.Message }

func callError(kind calltrace.ErrorKind, format string, args ...interface{}) error {
	return &CallError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

type RenderedRequest struct {
	Method  string
	URL     string
	Body    string
	Headers map[string]string
	Auth    HTTPAuth
	Secrets []string
}

func (c HTTPConfig) Parameters() (map[string]tools.Parameter, []string) {
	params := map[string]tools.Parameter{}
	var required []string
	for _, f := range c.ModelFields() {
		params[f.ArgName()] = fieldParameter(f)
		if f.Required {
			required = append(required, f.ArgName())
		}
	}
	sort.Strings(required)
	return params, required
}

func fieldParameter(f HTTPField) tools.Parameter {
	p := tools.Parameter{Type: f.Type, Description: f.Description, Enum: f.Enum}
	switch f.Type {
	case "object":
		items := tools.ParameterItems{Type: "object", Properties: map[string]tools.Parameter{}}
		for _, child := range f.Properties {
			items.Properties[child.Name] = fieldParameter(child)
			if child.Required {
				items.Required = append(items.Required, child.Name)
			}
		}
		p.Items = &items
	case "array":
		element := HTTPField{Type: "string"}
		if f.Items != nil {
			element = *f.Items
		}
		items := itemsOf(element)
		p.Items = &items
	}
	return p
}

func itemsOf(f HTTPField) tools.ParameterItems {
	items := tools.ParameterItems{Type: f.Type, Description: f.Description, Enum: f.Enum}
	switch f.Type {
	case "object":
		items.Properties = map[string]tools.Parameter{}
		for _, child := range f.Properties {
			items.Properties[child.Name] = fieldParameter(child)
			if child.Required {
				items.Required = append(items.Required, child.Name)
			}
		}
	case "array":
		element := HTTPField{Type: "string"}
		if f.Items != nil {
			element = *f.Items
		}
		nested := itemsOf(element)
		items.Items = &nested
	}
	return items
}

func (c HTTPConfig) CheckArguments(args map[string]interface{}) error {
	var problems []string
	for _, f := range c.ModelFields() {
		value, present := args[f.ArgName()]
		if !present || value == nil {
			if f.Required {
				problems = append(problems, fmt.Sprintf("%s é obrigatório", f.ArgName()))
			}
			continue
		}
		if problem := typeProblem(f, value); problem != "" {
			problems = append(problems, f.ArgName()+" "+problem)
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return callError(calltrace.ErrorKindArguments, "argumentos inválidos: %s. Corrija e chame de novo.", strings.Join(problems, "; "))
}

func typeProblem(f HTTPField, value interface{}) string {
	switch f.Type {
	case "string":
		s, ok := value.(string)
		if !ok {
			return "precisa ser texto"
		}
		if len(f.Enum) > 0 && !contains(f.Enum, s) {
			return "precisa ser um destes: " + strings.Join(f.Enum, ", ")
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return "precisa ser número"
		}
	case "integer":
		n, ok := value.(float64)
		if !ok || n != math.Trunc(n) {
			return "precisa ser número inteiro"
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return "precisa ser verdadeiro ou falso"
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return "precisa ser um objeto"
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return "precisa ser uma lista"
		}
	}
	return ""
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

type renderer struct {
	lookup  Lookup
	args    map[string]interface{}
	secrets []string
	failure error
}

func (r *renderer) resolve(scope, key string) (interface{}, bool) {
	if scope == ScopeArgs {
		value, ok := r.args[key]
		return value, ok
	}
	if r.lookup == nil {
		return nil, false
	}
	value, ok := r.lookup(scope, key)
	if ok && scope == ScopeSecret {
		r.secrets = append(r.secrets, value)
	}
	return value, ok
}

func (r *renderer) text(template string, allowArgs bool, escape func(string) string) string {
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := templatePattern.FindStringSubmatch(match)
		scope, key := parts[1], parts[2]
		if scope == ScopeArgs && !allowArgs {
			r.fail(calltrace.ErrorKindRender, "o modelo não preenche %s fora dos campos da IA", match)
			return ""
		}
		value, ok := r.resolve(scope, key)
		if !ok {
			r.missing(scope, match)
			return ""
		}
		text := fmt.Sprintf("%v", value)
		if escape != nil {
			return escape(text)
		}
		return text
	})
}

func (r *renderer) json(template string) string {
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := templatePattern.FindStringSubmatch(match)
		value, ok := r.resolve(parts[1], parts[2])
		if !ok {
			if parts[1] == ScopeArgs {
				return "null"
			}
			r.missing(parts[1], match)
			return "null"
		}
		raw, _ := json.Marshal(value)
		return string(raw)
	})
}

func (r *renderer) missing(scope, match string) {
	if scope == ScopeSecret {
		r.fail(calltrace.ErrorKindSecret, "o segredo de %s não existe neste workspace", match)
		return
	}
	r.fail(calltrace.ErrorKindRender, "não há valor para %s nesta conversa", match)
}

func (r *renderer) fail(kind calltrace.ErrorKind, format string, args ...interface{}) {
	if r.failure == nil {
		r.failure = callError(kind, format, args...)
	}
}

func (c HTTPConfig) Render(args map[string]interface{}, lookup Lookup) (RenderedRequest, error) {
	if err := c.CheckArguments(args); err != nil {
		return RenderedRequest{}, err
	}
	r := &renderer{lookup: lookup, args: args}
	out := RenderedRequest{Method: c.Method, Headers: map[string]string{}}

	base, query, fragment, hasQuery := splitQuery(c.URL)
	base = r.text(base, false, url.PathEscape)
	if hasQuery {
		query = r.text(query, false, url.QueryEscape)
	}
	extra := url.Values{}
	body := map[string]interface{}{}
	for _, f := range c.Fields {
		value, present := c.fieldValue(r, f)
		if !present {
			if f.Location == LocationPath {
				r.fail(calltrace.ErrorKindArguments, "falta o valor de %s para montar a URL", f.ArgName())
			}
			continue
		}
		switch f.Location {
		case LocationPath:
			base = strings.ReplaceAll(base, "{"+f.Name+"}", url.PathEscape(fmt.Sprintf("%v", value)))
		case LocationQuery:
			extra.Add(f.Name, fmt.Sprintf("%v", value))
		case LocationHeader:
			out.Headers[f.Name] = fmt.Sprintf("%v", value)
		case LocationBody:
			body[f.Name] = value
		}
	}
	if pathPlaceholderPattern.MatchString(base) {
		r.fail(calltrace.ErrorKindConfig, "a URL ainda tem campos sem valor: %s", pathPlaceholderPattern.FindString(base))
	}
	target := base
	if hasQuery || len(extra) > 0 {
		joined := query
		if encoded := extra.Encode(); encoded != "" {
			if joined != "" {
				joined += "&"
			}
			joined += encoded
		}
		target += "?" + joined
	}
	out.URL = target + fragment

	switch {
	case strings.TrimSpace(c.BodyTemplate) != "":
		out.Body = r.json(c.BodyTemplate)
		if r.failure == nil && !json.Valid([]byte(out.Body)) {
			r.fail(calltrace.ErrorKindRender, "o corpo montado não é um JSON válido")
		}
	case len(body) > 0:
		raw, _ := json.Marshal(body)
		out.Body = string(raw)
	}
	out.Auth = HTTPAuth{
		Mode:    c.Auth.Mode,
		User:    r.text(c.Auth.User, false, nil),
		Token:   r.text(c.Auth.Token, false, nil),
		KeyName: c.Auth.KeyName,
	}
	out.Secrets = r.secrets
	if r.failure != nil {
		return RenderedRequest{Secrets: r.secrets}, r.failure
	}
	return out, nil
}

func (c HTTPConfig) fieldValue(r *renderer, f HTTPField) (interface{}, bool) {
	switch f.Source {
	case SourceModel:
		value, ok := r.args[f.ArgName()]
		return value, ok && value != nil
	case SourceFixed:
		return f.Value, true
	default:
		return r.text(f.Value, false, nil), true
	}
}

func ValidOutputPath(path string) bool {
	return outputPathPattern.MatchString(strings.TrimSpace(path))
}

func (c HTTPConfig) MapOutput(status int, body []byte) (interface{}, error) {
	var parsed interface{}
	isJSON := json.Unmarshal(body, &parsed) == nil
	if len(c.Output.Fields) == 0 {
		if !isJSON {
			parsed = string(body)
		}
		if !c.Output.IncludeStatus {
			return parsed, nil
		}
		return map[string]interface{}{"status": status, "body": parsed}, nil
	}
	if !isJSON {
		return nil, callError(calltrace.ErrorKindMapping, "a resposta não é JSON, então os campos de saída não podem ser lidos")
	}
	out := map[string]interface{}{}
	for _, f := range c.Output.Fields {
		value, _ := valueAtPath(parsed, f.Path)
		out[f.Name] = value
	}
	if c.Output.IncludeStatus {
		out["status"] = status
	}
	return out, nil
}

func ValueAtPath(root interface{}, path string) (interface{}, bool) {
	if !ValidOutputPath(path) {
		return nil, false
	}
	return valueAtPath(root, path)
}

func valueAtPath(root interface{}, path string) (interface{}, bool) {
	current := root
	for _, m := range pathSegment.FindAllStringSubmatch(strings.TrimSpace(path), -1) {
		switch node := current.(type) {
		case map[string]interface{}:
			if m[1] == "" {
				return nil, false
			}
			next, ok := node[m[1]]
			if !ok {
				return nil, false
			}
			current = next
		case []interface{}:
			index, err := strconv.Atoi(m[2])
			if m[2] == "" || err != nil || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}
