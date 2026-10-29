package perception

import (
	"fmt"
	"math"
	"sort"
)

type AngleGroup struct {
	Degrees float64  `json:"degrees"`
	IDs     []string `json:"ids"`
}

var (
	sameAngle = 1.0
	nearAngle = 3.0
)

type angled struct {
	value float64
	id    string
}

func sortStrings(values []string) {
	sort.Strings(values)
}

func angleGroups(elements []Element) []AngleGroup {
	targets := make([]GuideTarget, len(elements))
	for i, el := range elements {
		targets[i] = GuideTarget{ID: el.ID, Transform: el.Transform}
	}
	return targetAngles(targets)
}

func targetAngles(targets []GuideTarget) []AngleGroup {
	var tilted []angled
	for _, target := range targets {
		if !isUpright(target.Transform) {
			tilted = append(tilted, angled{value: signedDegrees(target.Transform.Rotation), id: target.ID})
		}
	}
	stableSort(tilted, func(a, b angled) bool { return a.value < b.value })
	var groups [][]angled
	for _, item := range tilted {
		if n := len(groups); n > 0 && item.value-groups[n-1][0].value <= sameAngle {
			groups[n-1] = append(groups[n-1], item)
			continue
		}
		groups = append(groups, []angled{item})
	}
	out := make([]AngleGroup, len(groups))
	for i, group := range groups {
		sum := 0.0
		var ids []string
		for _, item := range group {
			sum += item.value
			ids = append(ids, item.id)
		}
		ids = uniqueStrings(ids)
		sortStrings(ids)
		out[i] = AngleGroup{Degrees: rounded(sum/float64(len(group)), 1), IDs: ids}
	}
	return out
}

func nearParallel(angles []AngleGroup) [][2]AngleGroup {
	var pairs [][2]AngleGroup
	for i, group := range angles {
		for _, other := range angles[i+1:] {
			apart := math.Abs(other.Degrees - group.Degrees)
			if apart > sameAngle && apart < nearAngle {
				pairs = append(pairs, [2]AngleGroup{group, other})
			}
		}
	}
	return pairs
}

func angleNotes(angles []AngleGroup) []string {
	notes := []string{}
	for _, pair := range nearParallel(angles) {
		notes = append(notes, fmt.Sprintf("%s a %s° e %s a %s° ficam quase paralelas; iguale o ângulo ou afaste de propósito", joinIDs(pair[0].IDs), num(pair[0].Degrees), joinIDs(pair[1].IDs), num(pair[1].Degrees)))
	}
	return notes
}

func Guides(targets []GuideTarget) GuideData {
	angles := targetAngles(targets)
	return GuideData{Angles: angles, Notes: angleNotes(angles)}
}
