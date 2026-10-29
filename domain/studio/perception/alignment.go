package perception

import (
	"math"
	"sort"
)

type layoutItem struct {
	el      Element
	box     Box
	corners []Point
	text    bool
	upright bool
	order   int
}

type alignmentMiss struct {
	off      *layoutItem
	edge     EdgeName
	distance float64
	partners []string
	members  []*layoutItem
	neighbor *layoutItem
}

type symmetryFound struct {
	axis  []*layoutItem
	pairs [][2]*layoutItem
}

var (
	backgroundShare = 0.7
	sameEdge        = 0.002
	artboardAxis    = 0.5
	minSymmetric    = 3
	mirrorSize      = 0.9
	mirrorAngle     = 0.5
	nearEdgeLow     = 0.004
	nearEdgeHigh    = 0.02
	containSlack    = 0.002
	decorOpacity    = 0.6
	edgeTouch       = 0.001
	maxSizeRatio    = 3.0
)

var axisEdges = map[EdgeName][]EdgeName{
	EdgeLeft:   {EdgeLeft, EdgeCenter, EdgeRight},
	EdgeCenter: {EdgeLeft, EdgeCenter, EdgeRight},
	EdgeRight:  {EdgeLeft, EdgeCenter, EdgeRight},
	EdgeTop:    {EdgeTop, EdgeMiddle, EdgeBottom},
	EdgeMiddle: {EdgeTop, EdgeMiddle, EdgeBottom},
	EdgeBottom: {EdgeTop, EdgeMiddle, EdgeBottom},
}

func bleeds(b Box) int {
	count := 0
	for _, touching := range []bool{b.Left <= edgeTouch, b.Right >= 1-edgeTouch, b.Top <= edgeTouch, b.Bottom >= 1-edgeTouch} {
		if touching {
			count++
		}
	}
	return count
}

func layoutItems(scene *Scene) []*layoutItem {
	var items []*layoutItem
	for _, el := range scene.Elements {
		if el.Opacity <= 0 || area(el.Box) >= backgroundShare || bleeds(el.Box) >= 2 {
			continue
		}
		item := &layoutItem{el: el, upright: isUpright(el.Transform), order: len(items)}
		if isText(el) {
			item.corners = inkCorners(scene.Canvas, el)
			item.box = unitBounds(item.corners, scene.Canvas)
			item.text = true
		} else {
			item.corners = cornersOf(el.Transform, scene.Canvas)
			item.box = el.Box
		}
		items = append(items, item)
	}
	return items
}

func contains(outer, inner Box) bool {
	return outer.Left <= inner.Left+containSlack && outer.Right >= inner.Right-containSlack && outer.Top <= inner.Top+containSlack && outer.Bottom >= inner.Bottom-containSlack
}

func nestedItems(a, b *layoutItem) bool {
	return contains(a.box, b.box) || contains(b.box, a.box)
}

func edgesOf(item *layoutItem) []EdgeName {
	if !item.text || item.el.Layer == nil {
		return edgeOrder
	}
	return []EdgeName{textAnchor(item.el.Layer), EdgeMiddle}
}

func hasEdge(item *layoutItem, edge EdgeName) bool {
	for _, e := range edgesOf(item) {
		if e == edge {
			return true
		}
	}
	return false
}

func sharesAxis(a, b *layoutItem, edge EdgeName) bool {
	for _, other := range axisEdges[edge] {
		if other != edge && hasEdge(a, other) && hasEdge(b, other) && math.Abs(edgeAt(other, a.box)-edgeAt(other, b.box)) <= sameEdge {
			return true
		}
	}
	return false
}

func aligns(item *layoutItem) bool {
	return item.upright && (item.text || item.el.Opacity >= decorOpacity)
}

func isVerticalEdge(edge EdgeName) bool {
	return edge == EdgeLeft || edge == EdgeCenter || edge == EdgeRight
}

func stackGap(a, b Box, edge EdgeName) (float64, float64) {
	if isVerticalEdge(edge) {
		if a.Bottom <= b.Top {
			return a.Bottom, b.Top
		}
		return b.Bottom, a.Top
	}
	if a.Right <= b.Left {
		return a.Right, b.Left
	}
	return b.Right, a.Left
}

func nearestOf(off *layoutItem, line []*layoutItem, edge EdgeName) *layoutItem {
	gap := func(item *layoutItem) float64 {
		from, to := stackGap(off.box, item.box, edge)
		return to - from
	}
	best := line[0]
	for _, item := range line[1:] {
		if gap(item) < gap(best) {
			best = item
		}
	}
	return best
}

func crossSize(item *layoutItem, edge EdgeName) float64 {
	if isVerticalEdge(edge) {
		return item.box.Right - item.box.Left
	}
	return item.box.Bottom - item.box.Top
}

func comparable(a, b *layoutItem, edge EdgeName) bool {
	small, large := crossSize(a, edge), crossSize(b, edge)
	if large < small {
		small, large = large, small
	}
	return large <= small*maxSizeRatio
}

func clearBetween(a, b *layoutItem, edge EdgeName, others []*layoutItem) bool {
	from, to := stackGap(a.box, b.box, edge)
	at := edgeAt(edge, b.box)
	for _, c := range others {
		if c == a || c == b {
			continue
		}
		low, high, start, end := c.box.Left, c.box.Right, c.box.Top, c.box.Bottom
		if isVerticalEdge(edge) {
			low, high, start, end = c.box.Top, c.box.Bottom, c.box.Left, c.box.Right
		}
		if low >= from && high <= to && start < at && end > at {
			return false
		}
	}
	return true
}

func related(off, neighbor *layoutItem, edge EdgeName, others []*layoutItem) bool {
	if edge != EdgeCenter && edge != EdgeMiddle {
		return true
	}
	return comparable(off, neighbor, edge) && clearBetween(off, neighbor, edge, others)
}

func onAxis(item *layoutItem) bool {
	return math.Abs(edgeAt(EdgeCenter, item.box)-artboardAxis) <= sameEdge
}

func similar(a, b float64) bool {
	return math.Min(a, b) >= math.Max(a, b)*mirrorSize
}

func mirrors(a, b *layoutItem) bool {
	if a.text != b.text || math.Abs(a.el.Rotation+b.el.Rotation) > mirrorAngle {
		return false
	}
	if math.Abs(edgeAt(EdgeCenter, a.box)+edgeAt(EdgeCenter, b.box)-2*artboardAxis) > sameEdge || math.Abs(edgeAt(EdgeMiddle, a.box)-edgeAt(EdgeMiddle, b.box)) > sameEdge {
		return false
	}
	return similar(a.box.Bottom-a.box.Top, b.box.Bottom-b.box.Top) && (a.text || similar(a.box.Right-a.box.Left, b.box.Right-b.box.Left))
}

func symmetry(items []*layoutItem) *symmetryFound {
	if len(items) < minSymmetric {
		return nil
	}
	var unpaired []*layoutItem
	found := &symmetryFound{}
	for _, item := range items {
		if onAxis(item) {
			found.axis = append(found.axis, item)
		} else {
			unpaired = append(unpaired, item)
		}
	}
	for len(unpaired) > 0 {
		first := unpaired[0]
		unpaired = unpaired[1:]
		at := -1
		for i, other := range unpaired {
			if mirrors(first, other) {
				at = i
				break
			}
		}
		if at < 0 {
			return nil
		}
		found.pairs = append(found.pairs, [2]*layoutItem{first, unpaired[at]})
		unpaired = append(append([]*layoutItem(nil), unpaired[:at]...), unpaired[at+1:]...)
	}
	return found
}

type placedEdge struct {
	item *layoutItem
	at   float64
}

func alignmentMisses(items []*layoutItem) []alignmentMiss {
	var upright []*layoutItem
	for _, item := range items {
		if aligns(item) {
			upright = append(upright, item)
		}
	}
	order := make(map[*layoutItem]int, len(upright))
	for i, item := range upright {
		order[item] = i
	}
	best := map[string]*alignmentMiss{}
	var bestKeys []string
	for _, edge := range edgeOrder {
		var sorted []placedEdge
		for _, item := range upright {
			if hasEdge(item, edge) {
				sorted = append(sorted, placedEdge{item: item, at: edgeAt(edge, item.box)})
			}
		}
		stableSort(sorted, func(x, y placedEdge) bool { return x.at < y.at })
		within := func(index int, low, high float64) []placedEdge {
			var found []placedEdge
			for j := index - 1; j >= 0 && sorted[index].at-sorted[j].at <= high; j-- {
				if sorted[index].at-sorted[j].at > low {
					found = append(found, sorted[j])
				}
			}
			for j := index + 1; j < len(sorted) && sorted[j].at-sorted[index].at <= high; j++ {
				if sorted[j].at-sorted[index].at > low {
					found = append(found, sorted[j])
				}
			}
			return found
		}
		partners := make([][]*layoutItem, len(sorted))
		place := make(map[*layoutItem]int, len(sorted))
		for i, entry := range sorted {
			for _, p := range within(i, -1, sameEdge) {
				partners[i] = append(partners[i], p.item)
			}
			place[entry.item] = i
		}
		for i, entry := range sorted {
			a := entry.item
			for _, candidate := range within(i, nearEdgeLow, nearEdgeHigh) {
				b := candidate.item
				if nestedItems(a, b) || sharesAxis(a, b, edge) {
					continue
				}
				bLine := partners[place[b]]
				aLine := partners[i]
				if len(aLine) > len(bLine) || (len(aLine) == len(bLine) && order[a] < order[b]) {
					continue
				}
				line := append([]*layoutItem{b}, bLine...)
				neighbor := nearestOf(a, line, edge)
				if !related(a, neighbor, edge, upright) {
					continue
				}
				distance := math.Abs(sorted[i].at - candidate.at)
				key := a.el.ID + ":" + string(edge)
				current, seen := best[key]
				if !seen || distance < current.distance {
					ids := make([]string, len(line))
					for k, c := range line {
						ids[k] = c.el.ID
					}
					sort.Strings(ids)
					if !seen {
						bestKeys = append(bestKeys, key)
					}
					best[key] = &alignmentMiss{off: a, edge: edge, distance: distance, partners: ids, members: line, neighbor: neighbor}
				}
			}
		}
	}
	byLayer := map[string]*alignmentMiss{}
	var layerKeys []string
	for _, key := range bestKeys {
		miss := best[key]
		current, seen := byLayer[miss.off.el.ID]
		if !seen {
			layerKeys = append(layerKeys, miss.off.el.ID)
		}
		if !seen || miss.distance < current.distance {
			byLayer[miss.off.el.ID] = miss
		}
	}
	misses := make([]alignmentMiss, 0, len(layerKeys))
	for _, key := range layerKeys {
		misses = append(misses, *byLayer[key])
	}
	return misses
}
