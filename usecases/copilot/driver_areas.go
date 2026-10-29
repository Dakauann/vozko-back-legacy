package copilot_usecase

import (
	"slices"

	"vozko/domain/ai"
	"vozko/domain/copilot"
)

func (d *Driver) withheld(tool copilot.Tool) bool {
	if !defersOn(d.cc.View) {
		return false
	}
	area := d.registry.areas.of(tool.Definition().Name)
	return area != "" && !d.loaded[area]
}

func (d *Driver) load(keys ...string) {
	for _, key := range keys {
		if d.registry.areas.known(key) && !d.loaded[key] {
			d.loaded[key] = true
			d.offered = nil
		}
	}
}

func (d *Driver) loadAreas(res copilot.Result, load areaLoad) copilot.Result {
	d.load(load.Areas...)
	load.Tools = []string{}
	for _, area := range d.registry.areas.areas {
		if !slices.Contains(load.Areas, area.Key) {
			continue
		}
		for _, name := range area.Tools {
			if tool, ok := d.registry.Get(name); ok && d.cc.View.Offers(tool) && d.permitTool(tool) == nil {
				load.Tools = append(load.Tools, name)
			}
		}
	}
	slices.Sort(load.Tools)
	res.Data = load
	return res
}

func (d *Driver) resume(conversations ...[]ai.Message) {
	for _, messages := range conversations {
		for _, m := range messages {
			for _, call := range m.ToolCalls {
				if call.Name == loadToolsName {
					d.load(requestedAreas(call.Arguments)...)
					continue
				}
				d.load(d.registry.areas.of(call.Name))
			}
		}
	}
}
