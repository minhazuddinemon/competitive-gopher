package ui

import (
	"fmt"
	"os"
	"os/exec"

	"go-cp-cli/cli-app/assets"
)

var chafaPath string
var chafaChecked bool

func chafaAvailable() bool {
	if !chafaChecked {
		if p, err := exec.LookPath("chafa"); err == nil {
			chafaPath = p
		}
		chafaChecked = true
	}
	return chafaPath != ""
}

// ClearKittyImages sends the kitty graphics protocol "delete all placements"
// command. Call this before clearing the screen so pixel images from the
// previous render don't bleed through into the new frame (kitty/sixel images
// live in a compositor layer that a plain "erase screen" escape doesn't touch).
func ClearKittyImages() {
	fmt.Print("\033_Ga=d\033\\")
}

// PrintImages renders two icons side-by-side as real pixel images using
// chafa's kitty graphics protocol output (requires Konsole 22.04+ or kitty
// terminal and chafa ≥ 1.12). leftName sits flush against the left edge of
// boxWidth, rightName flush against the right edge -- i.e. the pair is
// justified against the same width used for the box/dividers below them,
// rather than sitting close together with a fixed gap.
//
// It assumes the terminal cursor is at row 1 (i.e. the screen was just
// cleared). After both images are drawn it repositions the cursor to row
// rows+1 so the caller can print the header text immediately below.
//
// Returns true if images were actually rendered, false on any failure
// (chafa not found, icon not embedded, etc.) so the caller can print a
// plain-text fallback instead.
func PrintImages(leftName, rightName string, cols, rows, boxWidth int) bool {
	if !chafaAvailable() {
		return false
	}

	leftData, lErr := assets.Icon(leftName)
	rightData, rErr := assets.Icon(rightName)
	if lErr != nil && rErr != nil {
		return false
	}

	// printIcon writes one icon to a temp file and pipes chafa's kitty
	// output directly to stdout. Returns false if anything goes wrong.
	printIcon := func(data []byte, err error) bool {
		if err != nil || len(data) == 0 {
			return false
		}
		tmp, tErr := os.CreateTemp("", "cg-icon-*.png")
		if tErr != nil {
			return false
		}
		defer os.Remove(tmp.Name())
		if _, wErr := tmp.Write(data); wErr != nil {
			tmp.Close()
			return false
		}
		tmp.Close()

		cmd := exec.Command(chafaPath,
			"--size", fmt.Sprintf("%dx%d", cols, rows),
			// "kitty" instructs chafa to emit the Kitty graphics protocol
			// escape sequence, which Konsole renders as a real pixel image
			// rather than a grid of coloured Unicode block characters.
			"--format", "kitty",
			"--animate=off",
			tmp.Name(),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = nil
		return cmd.Run() == nil
	}

	// Gap between the two images so the right one's right edge lands on
	// boxWidth -- i.e. justified against the same width as the box/dividers
	// printed below, rather than a fixed distance between them.
	gap := max(boxWidth-2*cols, 2)

	// ── Left image ────────────────────────────────────────────────────────
	// After the kitty sequence is emitted, Konsole places the cursor at
	// the start of the row immediately below the image (row 1+rows, col 1).
	leftOK := printIcon(leftData, lErr)

	// ── Right image ────────────────────────────────────────────────────────
	// Move cursor back to the top of the image area, then step right past
	// the left image + gap so the second image lands beside the first.
	if leftOK {
		fmt.Printf("\033[%dA\033[%dC", rows, cols+gap)
	}
	printIcon(rightData, rErr)

	// ── Reposition cursor below both images ──────────────────────────────
	// Use absolute positioning so the header row that follows always starts
	// in the right place regardless of where the kitty renderer left the
	// cursor (different terminal versions behave slightly differently).
	fmt.Printf("\033[%d;1H", rows+1)
	return true
}
