package assets

import (
	"embed"
	"fmt"
)

// The go:embed directive below requires at least one matching file to
// exist at build time. Copy the four PNGs from the browser extension
// (cp-json-scraper/icons/) into cli-app/assets/icons/ with these exact
// names before building:
//
//	cli-app/assets/icons/gopher.png
//	cli-app/assets/icons/codeforces.png
//	cli-app/assets/icons/atcoder.png
//	cli-app/assets/icons/leetcode.png
//
//go:embed icons/*.png
var iconFS embed.FS

// Icon returns the raw PNG bytes for "gopher", "codeforces", "atcoder",
// or "leetcode".
func Icon(name string) ([]byte, error) {
	data, err := iconFS.ReadFile("icons/" + name + ".png")
	if err != nil {
		return nil, fmt.Errorf("icon %q not embedded (did you copy it into cli-app/assets/icons/?): %w", name, err)
	}
	return data, nil
}
