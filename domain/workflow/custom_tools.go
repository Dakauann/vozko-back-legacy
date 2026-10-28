package workflow

import "strings"

const customToolDefaultParamType = "string"

type CustomTool struct {
	Name        string
	Description string
	Parameters  []CustomToolParam
}

type CustomToolParam struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Enum        []string
}

var toolParamVariableTypes = map[string]VariableType{
	"string":   VariableTypeText,
	"number":   VariableTypeNumber,
	"integer":  VariableTypeNumber,
	"boolean":  VariableTypeBoolean,
	"array":    VariableTypeList,
	"object":   VariableTypeObject,
	"date":     VariableTypeDate,
	"time":     VariableTypeText,
	"datetime": VariableTypeText,
	"email":    VariableTypeText,
	"phone":    VariableTypeText,
	"enum":     VariableTypeText,
}

func (p CustomToolParam) VariableType() VariableType {
	if typ, ok := toolParamVariableTypes[p.Type]; ok {
		return typ
	}
	return VariableTypeUnknown
}

func CustomToolsOf(config map[string]interface{}) []CustomTool {
	raw, _ := config["custom_tools"].([]interface{})
	var out []CustomTool
	for _, item := range raw {
		m, _ := item.(map[string]interface{})
		name, _ := m["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		description, _ := m["description"].(string)
		tool := CustomTool{Name: name, Description: description}
		params, _ := m["parameters"].([]interface{})
		for _, rawParam := range params {
			if param, ok := customToolParamOf(rawParam); ok {
				tool.Parameters = append(tool.Parameters, param)
			}
		}
		out = append(out, tool)
	}
	return out
}

func customToolParamOf(raw interface{}) (CustomToolParam, bool) {
	m, _ := raw.(map[string]interface{})
	name, _ := m["name"].(string)
	if strings.TrimSpace(name) == "" {
		return CustomToolParam{}, false
	}
	typ, _ := m["type"].(string)
	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ == "" {
		typ = customToolDefaultParamType
	}
	description, _ := m["description"].(string)
	required, _ := m["required"].(bool)
	param := CustomToolParam{Name: name, Type: typ, Description: description, Required: required}
	rawEnum, _ := m["enum"].([]interface{})
	for _, e := range rawEnum {
		if s, ok := e.(string); ok && s != "" {
			param.Enum = append(param.Enum, s)
		}
	}
	return param, true
}
