package perception

import "math"

func crossesItem(guide Shape, item *layoutItem) bool {
	x0, y0, x1, y1 := guide.From[0], guide.From[1], guide.To[0], guide.To[1]
	b := item.box
	if x0 == x1 {
		return b.Left < x0 && x0 < b.Right && math.Min(y0, y1) < b.Bottom && math.Max(y0, y1) > b.Top
	}
	return b.Top < y0 && y0 < b.Bottom && math.Min(x0, x1) < b.Right && math.Max(x0, x1) > b.Left
}

func GuideCrossings(surface *Surface) (int, error) {
	scene := imageScene(surface)
	if err := scene.measured(); err != nil {
		return 0, err
	}
	items := layoutItems(scene)
	total := 0
	for _, miss := range alignmentMisses(items) {
		involved := append([]*layoutItem{miss.off, miss.neighbor}, miss.members...)
		var unrelated []*layoutItem
		for _, item := range items {
			related := false
			for _, member := range involved {
				if member == item || nestedItems(member, item) {
					related = true
					break
				}
			}
			if !related {
				unrelated = append(unrelated, item)
			}
		}
		for _, guide := range alignmentGuides(miss, items) {
			if guide.Kind != "guide" {
				continue
			}
			for _, item := range unrelated {
				if crossesItem(guide, item) {
					total++
					break
				}
			}
		}
	}
	return total, nil
}

func FindingLines(findings []Finding) []string {
	return findingLines(findings)
}
