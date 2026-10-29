package perception

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type golden struct {
	Source      string            `json:"source"`
	Fn          string            `json:"fn"`
	Args        goldenArgs        `json:"args"`
	Approximate bool              `json:"approximate"`
	Surface     *Surface          `json:"surface"`
	Video       *Video            `json:"video"`
	Legibility  []LegibilityEntry `json:"legibility"`
	Targets     []GuideTarget     `json:"targets"`
	Output      json.RawMessage   `json:"output"`
}

type goldenArgs struct {
	View   View   `json:"view"`
	Scene  string `json:"scene"`
	Layers Layers `json:"layers"`
}

type lookOutput struct {
	Data   map[string]any `json:"data"`
	Shapes any            `json:"shapes"`
}

func (g golden) run() (any, error) {
	certain := !g.Approximate
	switch g.Fn {
	case "imageChecks":
		return ImageChecks(g.Surface, certain)
	case "imageQrChecks":
		return ImageQRChecks(g.Surface), nil
	case "videoChecks":
		return VideoChecks(g.Video, certain)
	case "layout":
		if g.Args.Scene == "video" {
			return FrameLayout(g.Video, g.Video.Times[0], certain)
		}
		return ImageLayout(g.Surface, certain)
	case "depth":
		if g.Args.Scene == "video" {
			return FrameDepth(g.Video, g.Video.Times[0])
		}
		return ImageDepth(g.Surface), nil
	case "motion":
		return g.Video.MotionReport()
	case "imageLook":
		found, err := ImageLook(g.Args.View, g.Surface, certain)
		return lookOutput{Data: found.Data, Shapes: found.Shapes}, err
	case "videoLook":
		found, err := VideoLook(g.Args.View, g.Video, certain)
		return lookOutput{Data: found.Data, Shapes: found.Shapes}, err
	case "shapes":
		if g.Args.Scene == "video" {
			return FrameShapes(g.Video, g.Video.Times[0], g.Args.Layers, certain)
		}
		return ImageShapes(g.Surface, g.Args.Layers, certain)
	case "guides":
		angles := targetAngles(g.Targets)
		return GuideData{Angles: angles, Notes: angleNotes(angles)}, nil
	case "legibility":
		return Legibility(g.Surface.Canvas.Height, g.Legibility), nil
	}
	return nil, fmt.Errorf("unknown golden function %s", g.Fn)
}

func normalized(raw []byte) (any, error) {
	var value any
	err := json.Unmarshal(raw, &value)
	return value, err
}

func goldenFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "goldens", "*.jsonl.gz"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden files: %v", err)
	}
	return files
}

func readGoldens(t testing.TB, path string, each func(line int, raw []byte)) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var reader io.Reader = file
	zipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	reader = zipped
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	line := 0
	for scanner.Scan() {
		line++
		each(line, append([]byte(nil), scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestGoldensMatchTheBrowserAnalysis(t *testing.T) {
	for _, path := range goldenFiles(t) {
		name := strings.TrimSuffix(filepath.Base(path), ".jsonl.gz")
		t.Run(name, func(t *testing.T) {
			failures := 0
			readGoldens(t, path, func(line int, raw []byte) {
				var g golden
				if err := json.Unmarshal(raw, &g); err != nil {
					t.Fatalf("line %d: %v", line, err)
				}
				got, err := g.run()
				if err != nil {
					failures++
					if failures <= 5 {
						t.Errorf("line %d %s %s (%s): %v", line, g.Fn, g.Args.View, g.Source, err)
					}
					return
				}
				encoded, _ := json.Marshal(got)
				have, _ := normalized(encoded)
				want, _ := normalized(g.Output)
				var diffs []string
				differences("", normalizedAreas(want), normalizedAreas(have), &diffs)
				if len(diffs) > 0 {
					failures++
					if failures <= 5 {
						t.Errorf("line %d %s %s (%s):\n  %s", line, g.Fn, g.Args.View, g.Source, strings.Join(diffs, "\n  "))
					}
				}
			})
			if failures > 0 {
				t.Errorf("%d goldens differ", failures)
			}
		})
	}
}
