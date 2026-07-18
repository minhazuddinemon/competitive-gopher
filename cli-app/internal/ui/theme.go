package ui

import "github.com/charmbracelet/lipgloss"

// Platform brand colors. Codeforces blue and LeetCode orange are their
// well-documented site/logo colors. AtCoder doesn't have a strong single
// accent color (its site leans on near-black/navy text) -- #222222 is a
// reasonable stand-in; change it here if you'd rather use something else.
var (
	ColorCodeforces = lipgloss.Color("#1F8ACB")
	ColorAtCoder    = lipgloss.Color("#8FA6C4") // AtCoder's actual brand is near-black, which reads
	// as invisible/"dim" as a pill background or menu-highlight color on a dark
	// terminal (dark text on dark background). Lightened here for usability;
	// change it back to #222222 if you'd rather have exact brand accuracy over legibility.
	ColorLeetCode     = lipgloss.Color("#FFA116")
	ColorGopherAccent = lipgloss.Color("#00ADD8") // Go's own brand cyan, for app chrome

	ColorSuccess = lipgloss.Color("#50FA7B")
	ColorError   = lipgloss.Color("#FF5555")
	ColorWarning = lipgloss.Color("#FFB86C")
	ColorMuted   = lipgloss.Color("#6272A4")
	ColorText    = lipgloss.Color("#F8F8F2")
	ColorDarkBg  = lipgloss.Color("#0D1117") // dark text used ON TOP of bright badge/banner backgrounds
)

// PlatformTheme bundles everything the UI needs to render one platform
// consistently: its brand color, display name, and embedded icon key.
type PlatformTheme struct {
	Name     string
	Color    lipgloss.Color
	IconName string // key into assets.Icon(name)
}

var platforms = map[string]PlatformTheme{
	"codeforces": {Name: "Codeforces", Color: ColorCodeforces, IconName: "codeforces"},
	"atcoder":    {Name: "AtCoder", Color: ColorAtCoder, IconName: "atcoder"},
	"leetcode":   {Name: "LeetCode", Color: ColorLeetCode, IconName: "leetcode"},
}

// ThemeFor returns the theme for a platform key ("codeforces", "atcoder",
// "leetcode"), or a neutral fallback theme for anything else.
func ThemeFor(platform string) PlatformTheme {
	if t, ok := platforms[platform]; ok {
		return t
	}
	return PlatformTheme{Name: "Unknown", Color: ColorMuted, IconName: ""}
}

// Badge renders " PlatformName " as a bold pill filled with the platform's
// brand color, dark text on top for guaranteed contrast against any of the
// (now-lightened) brand colors above.
func (t PlatformTheme) Badge() string {
	return lipgloss.NewStyle().
		Foreground(ColorDarkBg).
		Background(t.Color).
		Bold(true).
		Padding(0, 1).
		Render(" " + t.Name + " ")
}
