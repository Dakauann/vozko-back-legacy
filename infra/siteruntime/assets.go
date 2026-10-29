package siteruntime

import "embed"

const (
	PowFile        = "pow.js"
	RuntimeFile    = "vz-runtime.js"
	BridgeFile     = "vz-editor-bridge.js"
	ScreenshotFile = "vz-screenshot.js"
	TailwindFile   = "vz-tailwind-browser.js"
)

//go:embed pow.js vz-runtime.js vz-editor-bridge.js vz-screenshot.js vz-tailwind-browser.js
var Assets embed.FS

func Names() []string {
	return []string{RuntimeFile, PowFile, BridgeFile, ScreenshotFile, TailwindFile}
}

func Files() (map[string]string, error) {
	files := map[string]string{}
	for _, name := range Names() {
		raw, err := Assets.ReadFile(name)
		if err != nil {
			return nil, err
		}
		files[name] = string(raw)
	}
	return files, nil
}
