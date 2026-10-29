package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"vozko/domain/studio/perception"
)

const maxLineBytes = 64 << 20

type request struct {
	ID    int             `json:"id"`
	Op    string          `json:"op"`
	Facts json.RawMessage `json:"facts"`
}

type answer struct {
	ID     int    `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type frameReports struct {
	AtMS   float64                `json:"at_ms"`
	Depth  perception.DepthReport  `json:"depth"`
	Layout perception.LayoutReport `json:"layout"`
}

var errUnknownOp = errors.New("unknown op")

func imageReports(facts *perception.Facts) (any, error) {
	if facts.Surface == nil {
		return nil, perception.ErrIncomplete
	}
	layout, err := perception.ImageLayout(facts.Surface, !facts.Approximate)
	if err != nil {
		return nil, err
	}
	return map[string]any{"depth": perception.ImageDepth(facts.Surface), "layout": layout}, nil
}

func videoFrames(facts *perception.Facts) (any, error) {
	if facts.Video == nil {
		return nil, perception.ErrIncomplete
	}
	out := make([]frameReports, 0, len(facts.Video.Times))
	for _, at := range facts.Video.Times {
		depth, err := perception.FrameDepth(facts.Video, at)
		if err != nil {
			return nil, err
		}
		layout, err := perception.FrameLayout(facts.Video, at, !facts.Approximate)
		if err != nil {
			return nil, err
		}
		out = append(out, frameReports{AtMS: at, Depth: depth, Layout: layout})
	}
	return out, nil
}

func perceive(op string, facts *perception.Facts) (any, error) {
	switch op {
	case "checks":
		return perception.Checks(facts)
	case "look":
		return perception.Look(facts)
	case "image":
		return imageReports(facts)
	case "frames":
		return videoFrames(facts)
	case "motion":
		if facts.Video == nil {
			return nil, perception.ErrIncomplete
		}
		report, err := facts.Video.MotionReport()
		if err != nil {
			return nil, err
		}
		return map[string]any{"moves": report.Moves, "variety": report.Variety, "findings": report.Findings, "lines": perception.FindingLines(report.Findings)}, nil
	case "crossings":
		if facts.Surface == nil {
			return nil, perception.ErrIncomplete
		}
		return perception.GuideCrossings(facts.Surface)
	case "legibility":
		if facts.Surface == nil {
			return nil, perception.ErrIncomplete
		}
		return perception.Legibility(facts.Surface.Canvas.Height, facts.Legibility), nil
	case "guides":
		return perception.Guides(facts.Targets), nil
	}
	return nil, fmt.Errorf("%w %q", errUnknownOp, op)
}

func handle(line []byte) answer {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return answer{Error: err.Error()}
	}
	facts, err := perception.DecodeFacts(req.Facts)
	if err != nil {
		return answer{ID: req.ID, Error: err.Error()}
	}
	result, err := perceive(req.Op, facts)
	if err != nil {
		return answer{ID: req.ID, Error: err.Error()}
	}
	return answer{ID: req.ID, Result: result}
}

func serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 1<<20), maxLineBytes)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		if err := encoder.Encode(handle(scanner.Bytes())); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func main() {
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
