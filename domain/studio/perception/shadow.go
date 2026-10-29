package perception

import (
	"math"

	"vozko/domain/studio"
)

type castKind string

const (
	castDrop castKind = "drop"
	castGlow castKind = "glow"
)

type shadowCast struct {
	layerID  string
	effectID string
	kind     castKind
	dx, dy   float64
	distance float64
	angle    float64
	blur     float64
	spread   float64
	alpha    float64
	color    Rgba
	side     float64
}

type lightOutlier struct {
	cast  *shadowCast
	light float64
}

var (
	minCastDistance = 2.0
	outlierDegrees  = 40.0
	minAgreement    = 0.8
	showsRatio      = 1.1
)

func degrees(dx, dy float64) float64 {
	return math.Atan2(dy, dx) * 180 / math.Pi
}

func angleApart(a, b float64) float64 {
	turn := math.Mod(math.Abs(math.Mod(a-b, 360)+360), 360)
	return math.Min(turn, 360-turn)
}

func layerEffects(layer *studio.Layer) []studio.Effect {
	if len(layer.Effects) > 0 {
		var visible []studio.Effect
		for _, effect := range layer.Effects {
			if !effect.Hidden {
				visible = append(visible, effect)
			}
		}
		return visible
	}
	if layer.Shadow != nil {
		return []studio.Effect{{ID: "shadow", Type: "drop-shadow", Color: layer.Shadow.Color, Opacity: 1, X: layer.Shadow.X, Y: layer.Shadow.Y, Blur: layer.Shadow.Blur, ShowBehind: true}}
	}
	return nil
}

func worldOffset(layer *studio.Layer, x, y float64, styled bool) (float64, float64) {
	if !styled {
		return x, y
	}
	if layer.FlipX && layer.Type != studio.LayerImage {
		x = -x
	}
	if layer.FlipY && layer.Type != studio.LayerImage {
		y = -y
	}
	radians := radiansOf(layer.Transform.Rotation)
	cos, sin := math.Cos(radians), math.Sin(radians)
	return x*cos - y*sin, x*sin + y*cos
}

func shadowCasts(layer *studio.Layer, canvas Canvas) []*shadowCast {
	styled := len(layer.Effects) > 0
	side := math.Min(layer.Transform.W*canvas.Width, layer.Transform.H*canvas.Height)
	var casts []*shadowCast
	for _, effect := range layerEffects(layer) {
		if effect.Type != "drop-shadow" && effect.Type != "outer-glow" {
			continue
		}
		color := parseColor(effect.Color)
		if color == nil {
			continue
		}
		dx, dy := worldOffset(layer, effect.X, effect.Y, styled)
		kind := castGlow
		if effect.Type == "drop-shadow" {
			kind = castDrop
		}
		solidColor := *color
		solidColor.A = 1
		casts = append(casts, &shadowCast{
			layerID: layer.ID, effectID: effect.ID, kind: kind, dx: dx, dy: dy, distance: jsHypot(dx, dy), angle: degrees(dx, dy),
			blur: effect.Blur, spread: effect.Spread, alpha: color.A * effect.Opacity * layer.Transform.Opacity, color: solidColor, side: side,
		})
	}
	return casts
}

func directed(casts []*shadowCast) []*shadowCast {
	var usable []*shadowCast
	for _, cast := range casts {
		if cast.kind == castDrop && cast.distance >= minCastDistance {
			usable = append(usable, cast)
		}
	}
	return usable
}

func meanDirection(casts []*shadowCast) (float64, float64) {
	x, y := 0.0, 0.0
	for _, cast := range casts {
		x += cast.dx / cast.distance
	}
	for _, cast := range casts {
		y += cast.dy / cast.distance
	}
	n := float64(len(casts))
	x, y = x/n, y/n
	return degrees(x, y), jsHypot(x, y)
}

type lightReading struct {
	angle     float64
	agreement float64
	count     int
}

func lightDirection(casts []*shadowCast) *lightReading {
	usable := directed(casts)
	if len(usable) == 0 {
		return nil
	}
	angle, agreement := meanDirection(usable)
	return &lightReading{angle: angle, agreement: agreement, count: len(usable)}
}

func lightOutliers(casts []*shadowCast) []lightOutlier {
	usable := directed(casts)
	if len(usable) < 3 {
		return nil
	}
	var outliers []lightOutlier
	for _, cast := range usable {
		var others []*shadowCast
		for _, other := range usable {
			if other != cast {
				others = append(others, other)
			}
		}
		angle, agreement := meanDirection(others)
		if agreement >= minAgreement && angleApart(cast.angle, angle) > outlierDegrees {
			outliers = append(outliers, lightOutlier{cast: cast, light: angle})
		}
	}
	return outliers
}

func shadowShows(shade Rgba, alpha float64, ground Rgba) bool {
	return contrastRatio(mixColors(ground, shade, alpha), ground) >= showsRatio
}
