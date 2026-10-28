package site_usecase

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"vozko/domain/site"
	"vozko/domain/strictjson"
)

//go:embed all:starters
var starterFiles embed.FS

const (
	starterRoot     = "starters"
	starterMetaFile = "starter.json"
	starterFilesDir = "files"

	StarterPreviewFile = "preview.jpg"
)

type Starter struct {
	Stack       site.Stack
	Key         string
	Title       string
	Description string
	Settings    site.Settings
	Files       map[string]string
	Preview     []byte
}

func (s Starter) Project() site.Project {
	return site.NewProject(s.Stack, s.Files)
}

type starterMeta struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Settings    json.RawMessage `json:"settings"`
}

var (
	starterOnce sync.Once
	starterList []Starter
	starterErr  error
)

func Starters() ([]Starter, error) {
	starterOnce.Do(func() {
		starterList, starterErr = loadStarters(starterFiles, true)
	})
	if starterErr != nil {
		return nil, starterErr
	}
	return slices.Clone(starterList), nil
}

func StarterFor(stack site.Stack, key string) (Starter, error) {
	all, err := Starters()
	if err != nil {
		return Starter{}, err
	}
	for _, s := range all {
		if s.Stack == stack && s.Key == key {
			return s, nil
		}
	}
	return Starter{}, fmt.Errorf("%w: %s/%s", ErrStarterUnknown, stack, key)
}

func StarterSources() ([]Starter, error) {
	return loadStarters(starterFiles, false)
}

func loadStarters(root fs.FS, withPreview bool) ([]Starter, error) {
	var out []Starter
	for _, stack := range site.Stacks() {
		entries, err := fs.ReadDir(root, path.Join(starterRoot, string(stack)))
		if err != nil {
			return nil, fmt.Errorf("site starters: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			starter, err := loadStarter(root, stack, entry.Name(), withPreview)
			if err != nil {
				return nil, err
			}
			out = append(out, starter)
		}
	}
	slices.SortFunc(out, func(a, b Starter) int {
		return strings.Compare(string(a.Stack)+"/"+a.Key, string(b.Stack)+"/"+b.Key)
	})
	return out, nil
}

func loadStarter(root fs.FS, stack site.Stack, key string, withPreview bool) (Starter, error) {
	dir := path.Join(starterRoot, string(stack), key)
	raw, err := fs.ReadFile(root, path.Join(dir, starterMetaFile))
	if err != nil {
		return Starter{}, fmt.Errorf("site starter %s/%s: %w", stack, key, err)
	}
	var meta starterMeta
	if err := strictjson.Decode(raw, &meta); err != nil {
		return Starter{}, fmt.Errorf("site starter %s/%s: %w", stack, key, err)
	}
	settings, err := site.DecodeSettings(meta.Settings)
	if err != nil {
		return Starter{}, fmt.Errorf("site starter %s/%s: %w", stack, key, err)
	}
	files := map[string]string{}
	filesDir := path.Join(dir, starterFilesDir)
	err = fs.WalkDir(root, filesDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		content, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(p, filesDir+"/")] = strings.ReplaceAll(string(content), "\r\n", "\n")
		return nil
	})
	if err != nil {
		return Starter{}, fmt.Errorf("site starter %s/%s: %w", stack, key, err)
	}
	starter := Starter{Stack: stack, Key: key, Title: meta.Title, Description: meta.Description, Settings: settings, Files: files}
	if !withPreview {
		return starter, nil
	}
	preview, err := fs.ReadFile(root, path.Join(dir, StarterPreviewFile))
	if err != nil || len(preview) == 0 {
		return Starter{}, fmt.Errorf("site starter %s/%s: the preview image is missing: %w", stack, key, err)
	}
	starter.Preview = preview
	return starter, nil
}
