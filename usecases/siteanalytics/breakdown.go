package siteanalytics_usecase

import (
	"cmp"
	"slices"

	"vozko/domain/sitepublish"
)

func conversions(formSubmits, whatsapp int64) int64 {
	return formSubmits + whatsapp
}

func conversionRate(t Totals) int64 {
	base := t.Visitors
	if base <= 0 {
		base = t.Views
	}
	if base <= 0 {
		return 0
	}
	return min(t.Conversions, base) * basisPoints / base
}

func breakdown(days []sitepublish.DailyStat, key func(sitepublish.DailyStat) string) []Breakdown {
	rows := map[string]*Breakdown{}
	out := []Breakdown{}
	var order []string
	for _, d := range days {
		k := key(d)
		row, ok := rows[k]
		if !ok {
			row = &Breakdown{Key: k}
			rows[k] = row
			order = append(order, k)
		}
		row.Views += d.Views
		row.Visitors += d.Visitors
		row.CTA += d.CTA
		row.FormSubmits += d.FormSubmits
		row.WhatsApp += d.WhatsApp
		row.Conversions += conversions(d.FormSubmits, d.WhatsApp)
	}
	for _, k := range order {
		out = append(out, *rows[k])
	}
	slices.SortFunc(out, func(a, b Breakdown) int {
		return cmp.Or(cmp.Compare(b.Views, a.Views), cmp.Compare(a.Key, b.Key))
	})
	return out
}

func daily(days []sitepublish.DailyStat) []DailyPoint {
	points := map[string]*DailyPoint{}
	out := []DailyPoint{}
	for _, d := range days {
		key := d.Day.UTC().Format(dateLayout)
		point, ok := points[key]
		if !ok {
			point = &DailyPoint{Day: key}
			points[key] = point
		}
		point.Views += d.Views
		point.Visitors += d.Visitors
		point.Conversions += conversions(d.FormSubmits, d.WhatsApp)
	}
	for _, point := range points {
		out = append(out, *point)
	}
	slices.SortFunc(out, func(a, b DailyPoint) int { return cmp.Compare(a.Day, b.Day) })
	return out
}
