package tools

import "strings"

type InstanceKind string

const (
	InstancesSingle    InstanceKind = "single"
	InstancesPerTarget InstanceKind = "per_target"
	InstancesMany      InstanceKind = "many"
)

type InstancePolicy struct {
	Kind InstanceKind
	Key  string
}

func PerTarget(key string) InstancePolicy {
	return InstancePolicy{Kind: InstancesPerTarget, Key: key}
}

func Many() InstancePolicy {
	return InstancePolicy{Kind: InstancesMany}
}

func (p InstancePolicy) normalized() InstancePolicy {
	if p.Kind == "" {
		p.Kind = InstancesSingle
	}
	return p
}

func (p InstancePolicy) AllowsMany() bool {
	return p.normalized().Kind != InstancesSingle
}

func (p InstancePolicy) IsPerTarget() bool {
	return p.normalized().Kind == InstancesPerTarget && p.Key != ""
}

func (d Definition) TargetOf(config map[string]interface{}) string {
	if !d.Instances.IsPerTarget() {
		return ""
	}
	value, _ := config[d.Instances.Key].(string)
	return strings.TrimSpace(value)
}

func (d Definition) LegacyName(config map[string]interface{}) string {
	if !d.Instances.IsPerTarget() {
		return strings.ToLower(strings.TrimSpace(d.Name))
	}
	return BoundToolName(d.Name, d.TargetOf(config))
}

func (p InstancePolicy) Effective() InstanceKind {
	return p.normalized().Kind
}

type InstanceRef struct {
	ID       string
	Type     string
	Revision int
}
