package perception

import (
	"math"

	"vozko/domain/studio"
)

type Point [2]float64

type Box struct {
	Left, Top, Right, Bottom float64
}

type Canvas struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func signedArea(points []Point) float64 {
	twice := 0.0
	for i := range points {
		a, b := points[i], points[(i+1)%len(points)]
		twice += a[0]*b[1] - b[0]*a[1]
	}
	return twice / 2
}

func convexArea(points []Point) float64 {
	return math.Abs(signedArea(points))
}

func sideOf(a, b, p Point) float64 {
	return (b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0])
}

func crossingPoint(a, b, p, q Point) Point {
	sp := sideOf(a, b, p)
	sq := sideOf(a, b, q)
	t := sp / (sp - sq)
	return Point{p[0] + (q[0]-p[0])*t, p[1] + (q[1]-p[1])*t}
}

func winding(polygon []Point) float64 {
	area := signedArea(polygon)
	switch {
	case area > 0:
		return 1
	case area < 0:
		return -1
	}
	return 1
}

func insideConvex(point Point, polygon []Point) bool {
	turn := winding(polygon)
	for i, a := range polygon {
		if sideOf(a, polygon[(i+1)%len(polygon)], point)*turn < 0 {
			return false
		}
	}
	return true
}

func convexIntersection(subject, clip []Point) []Point {
	turn := winding(clip)
	output := append([]Point(nil), subject...)
	for i := 0; i < len(clip) && len(output) > 0; i++ {
		a, b := clip[i], clip[(i+1)%len(clip)]
		input := output
		output = nil
		for j := range input {
			p, q := input[j], input[(j+1)%len(input)]
			pIn := sideOf(a, b, p)*turn >= 0
			qIn := sideOf(a, b, q)*turn >= 0
			if pIn {
				output = append(output, p)
			}
			if pIn != qIn {
				output = append(output, crossingPoint(a, b, p, q))
			}
		}
	}
	if convexArea(output) > 0 {
		return output
	}
	return nil
}

func radiansOf(degrees float64) float64 {
	return degrees * math.Pi / 180
}

func boxCorners(t studio.Transform, canvas Canvas) []Point {
	halfW := t.W * canvas.Width / 2
	halfH := t.H * canvas.Height / 2
	radians := radiansOf(t.Rotation)
	cos, sin := math.Cos(radians), math.Sin(radians)
	cx, cy := t.X*canvas.Width, t.Y*canvas.Height
	offsets := [4][2]float64{{-halfW, -halfH}, {halfW, -halfH}, {halfW, halfH}, {-halfW, halfH}}
	corners := make([]Point, 4)
	for i, o := range offsets {
		corners[i] = Point{cx + o[0]*cos - o[1]*sin, cy + o[0]*sin + o[1]*cos}
	}
	return corners
}

func boxBounds(t studio.Transform, canvas Canvas) Box {
	w := t.W * canvas.Width
	h := t.H * canvas.Height
	radians := radiansOf(t.Rotation)
	cos, sin := math.Abs(math.Cos(radians)), math.Abs(math.Sin(radians))
	halfW := (w*cos + h*sin) / 2
	halfH := (w*sin + h*cos) / 2
	cx, cy := t.X*canvas.Width, t.Y*canvas.Height
	return Box{Left: cx - halfW, Top: cy - halfH, Right: cx + halfW, Bottom: cy + halfH}
}

func unitBoxOf(t studio.Transform, canvas Canvas) Box {
	radians := radiansOf(t.Rotation)
	cos, sin := math.Abs(math.Cos(radians)), math.Abs(math.Sin(radians))
	halfW := t.W * canvas.Width / 2
	halfH := t.H * canvas.Height / 2
	spanX := (cos*halfW + sin*halfH) / canvas.Width
	spanY := (sin*halfW + cos*halfH) / canvas.Height
	return Box{Left: t.X - spanX, Right: t.X + spanX, Top: t.Y - spanY, Bottom: t.Y + spanY}
}

func area(b Box) float64 {
	return math.Max(0, b.Right-b.Left) * math.Max(0, b.Bottom-b.Top)
}

func intersection(a, b Box) float64 {
	return area(Box{Left: math.Max(a.Left, b.Left), Top: math.Max(a.Top, b.Top), Right: math.Min(a.Right, b.Right), Bottom: math.Min(a.Bottom, b.Bottom)})
}

func unitBounds(points []Point, canvas Canvas) Box {
	box := Box{Left: math.Inf(1), Top: math.Inf(1), Right: math.Inf(-1), Bottom: math.Inf(-1)}
	for _, p := range points {
		x, y := p[0]/canvas.Width, p[1]/canvas.Height
		box.Left, box.Right = math.Min(box.Left, x), math.Max(box.Right, x)
		box.Top, box.Bottom = math.Min(box.Top, y), math.Max(box.Bottom, y)
	}
	return box
}

func sharedArea(a, b []Point) float64 {
	return convexArea(convexIntersection(a, b))
}

func overlapShare(a, b []Point) float64 {
	return sharedArea(a, b) / math.Max(1, math.Min(convexArea(a), convexArea(b)))
}

func signedDegrees(rotation float64) float64 {
	return math.Mod(math.Mod(rotation+180, 360)+360, 360) - 180
}

func isUpright(t studio.Transform) bool {
	turn := math.Mod(math.Abs(signedDegrees(t.Rotation)), 90)
	return math.Min(turn, 90-turn) < 0.5
}
