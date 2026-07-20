package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"go-cp-cli/cli-app/internal/parser"
	"go-cp-cli/cli-app/internal/runner"
	"go-cp-cli/cli-app/internal/ui"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Define Menu Item Structure
type MenuItem struct {
	Key   string
	Label string
}

// exitApp restores the normal screen buffer before exiting. Plain os.Exit
// skips deferred functions, so every exit path in this program must go
// through here instead -- otherwise the terminal gets left on the
// alternate screen buffer after the process dies.
func exitApp(code int) {
	fmt.Print("\x1b[?1049l")
	os.Exit(code)
}

func main() {
	// Alternate screen buffer: isolates this app's redraws from the user's
	// normal shell scrollback. Combined with chafa's --format symbols (see
	// image.go), this is what stops logos from visually "stacking" across
	// re-runs -- sixel/kitty graphics on some terminals (Konsole included)
	// live in a compositor layer that a plain "erase screen" doesn't
	// reliably clear; symbols are ordinary text and the alt screen gives
	// every redraw a clean slate regardless.
	fmt.Print("\x1b[?1049h")

	// Base System Color Palettes (still used for compile/menu chrome that
	// isn't part of the per-platform test-case rendering, which goes
	// through internal/ui instead).
	errorColor := lipgloss.Color("#FF5555")   // High-vis Red
	successColor := lipgloss.Color("#50FA7B") // Electric Green
	warningColor := lipgloss.Color("#FFB86C") // Bright Orange

	if len(os.Args) < 2 {
		errStyle := lipgloss.NewStyle().Foreground(errorColor).Bold(true)
		fmt.Println(errStyle.Render("🚨 Error: Missing solution file operand."))
		exitApp(1)
	}
	solutionFile := os.Args[len(os.Args)-1]

	// Persistent Memory State
	var probData *parser.ProblemData

	// Sub-function to load/reload problem data from clipboard
	loadFromClipboard := func() bool {
		rawClipboard, _ := clipboard.ReadAll()
		parsed, err := parser.ParseClipboardJSON(rawClipboard)
		if err != nil {
			fmt.Printf("\n%s %v\n", lipgloss.NewStyle().Foreground(errorColor).Bold(true).Render("🚨 Verification Failure:"), err)
			return false
		}
		probData = parsed
		if probData.TimeLimitMs == 0 {
			probData.TimeLimitMs = 2000
		}
		return true
	}

	// Load initial problem
	if !loadFromClipboard() {
		fmt.Println("Please copy a valid problem payload to clipboard and restart.")
		exitApp(1)
	}

	// Track active selection index for the menu
	menuCursorIndex := 0

	// Main Persistent State Machine Loop
	for {
		// Rebuilt every iteration (not once, before the loop) because
		// "Reload from Clipboard" can change probData.Platform/ProblemID
		// mid-session, and Run All Test Cases should only appear when a
		// local CSES test bank actually exists for the current problem.
		menuItems := []MenuItem{
			{Key: "run", Label: "Run Again"},
			{Key: "add", Label: "Add Custom Test Case"},
			{Key: "remove", Label: "Remove Test Case"},
		}
		if probData.Platform == "cses" && runner.HasCSESTestBank(probData.ProblemID) {
			menuItems = append(menuItems, MenuItem{Key: "runall", Label: "Run All Test Cases"})
		}
		menuItems = append(menuItems,
			MenuItem{Key: "reload", Label: "Reload from Clipboard"},
			MenuItem{Key: "exit", Label: "Exit"},
		)
		if menuCursorIndex >= len(menuItems) {
			menuCursorIndex = 0
		}

		// Clear kitty pixel images from the previous frame before erasing
		// the screen. Kitty/sixel graphics live in a compositor layer that
		// a plain "erase screen" escape doesn't touch, so without this they
		// visually pile up across re-runs in Konsole and kitty terminal.
		ui.ClearKittyImages()
		// Clear terminal screen for a clean UI update per execution cycle
		fmt.Print("\033[H\033[2J")

		// Logos only for now -- the title/constraint/compile-status row
		// (ui.RenderHeaderInfoRow) prints once we actually know whether
		// compilation succeeded, further down.
		ui.RenderHeaderImages(probData.Platform)

		// 1. Compile Phase (silent until the outcome is known -- the
		// target file + DONE/FAILED status now print together with the title
		// row via ui.RenderHeaderInfoRow once we actually have a result)
		var binaryPath string
		var leetcodeCleanSource string
		var err error

		if probData.Platform == "leetcode" {
			functionSignature := probData.FunctionSignature
			if functionSignature == "" {
				functionSignature = "func twoSum(nums []int, target int) []int"
			}

			details, sigErr := runner.ParseLeetCodeSignature(functionSignature)
			if sigErr != nil {
				ui.RenderHeaderInfoRow(probData.Platform, probData.Title, probData.TimeLimitMs, false)
				fmt.Printf("\nSignature Error: %v\n", sigErr)
				goto ShowMenu
			}

			// Format unstructured inputs into JSON objects
			var rawCases []string
			var rawExpecteds []string
			for _, t := range probData.Tests {
				// ConvertInputToJSONMap already locates each "paramName = value" pair
				// itself by searching for the "paramName =" prefix — pass the raw
				// scraped input straight through instead of pre-stripping it, or the
				// prefix search below has nothing left to find.
				jsonMapStr, convErr := runner.ConvertInputToJSONMap(t.Input, details.Params)
				if convErr != nil {
					ui.RenderHeaderInfoRow(probData.Platform, probData.Title, probData.TimeLimitMs, false)
					fmt.Printf("\nInput Parsing Error: %v\n", convErr)
					goto ShowMenu
				}
				rawCases = append(rawCases, jsonMapStr)
				rawExpecteds = append(rawExpecteds, t.Expected)
			} // Build and compile the hidden sandbox
			sandboxDir, cleanedSource, prepErr := runner.PrepareLeetCodeSandbox(
				solutionFile, "", "", probData.OrderMatters, probData.InPlace, probData.TargetParam, details, rawCases, rawExpecteds,
			)
			if prepErr != nil {
				ui.RenderHeaderInfoRow(probData.Platform, probData.Title, probData.TimeLimitMs, false)
				fmt.Printf("\nSandbox Build Error: %v\n", prepErr)
				goto ShowMenu
			}
			leetcodeCleanSource = cleanedSource

			binaryPath, err = runner.CompileLeetCodeSandbox(sandboxDir)
			if err != nil {
				ui.RenderHeaderInfoRow(probData.Platform, probData.Title, probData.TimeLimitMs, false)
				fmt.Printf("\n%s\n", err.Error())

				// =========================================================================
				// 🔍 TEMPORARY DIAGNOSTIC PRINTER: Print out the generated file with line numbers
				// =========================================================================
				if genContent, readErr := os.ReadFile(sandboxDir + "/runner_main.go"); readErr == nil {
					fmt.Println(lipgloss.NewStyle().Foreground(warningColor).Bold(true).Render("\n--- DEBUG: Malformed runner_main.go Content ---"))
					lines := strings.Split(string(genContent), "\n")
					for i, line := range lines {
						// Highlights line 12 specifically to make it easy to see
						if i+1 == 12 {
							fmt.Printf("🔴 %3d | %s\n", i+1, line)
						} else {
							fmt.Printf("   %3d | %s\n", i+1, line)
						}
					}
					fmt.Println(strings.Repeat("─", 65))
				}
				// =========================================================================

				goto ShowMenu
			}

		} else {
			// Standard Codeforces / AtCoder Compilation
			binaryPath, err = runner.CompileSolution(solutionFile)
		}

		if err != nil {
			ui.RenderHeaderInfoRow(probData.Platform, probData.Title, probData.TimeLimitMs, false)
			fmt.Printf("\n%s\n", err.Error())
			goto ShowMenu
		}

		ui.RenderHeaderInfoRow(probData.Platform, probData.Title, probData.TimeLimitMs, true)

		// 2. Test Execution Engine — collect all outcomes first, then
		// render the entire block at once so it can be wrapped in one box.
		{
			allPassed := true
			var outcomes []ui.CaseOutcome

			if probData.Platform == "leetcode" {
				// Wrap LeetCode sandbox execution with systemd-run limits
				cmd := exec.Command("systemd-run",
					"--user",
					"--scope",
					"-q",
					"-p", fmt.Sprintf("MemoryMax=%dM", probData.MemoryLimitMb),
					binaryPath,
				)
				output, runErr := cmd.CombinedOutput()
				outputStr := string(output)

				// If the binary produced literally no output, the harness never
				// even reached its first Printf — either it failed to launch at
				// all (bad binary path, missing exec permission, etc.) or it
				// panicked before printing anything. Either way, runErr (which
				// was previously discarded) is the only place that information
				// exists, so surface it instead of silently showing an empty box.
				if strings.TrimSpace(outputStr) == "" {
					allPassed = false
					fmt.Println(lipgloss.NewStyle().Foreground(errorColor).Bold(true).
						Render("🚨 Sandbox binary produced no output at all."))
					if runErr != nil {
						if strings.Contains(runErr.Error(), "signal: killed") {
							fmt.Printf("   Execution error: MEMORY LIMIT EXCEEDED (> %d MB)\n", probData.MemoryLimitMb)
						} else {
							fmt.Printf("   Execution error: %v\n", runErr)
						}
					} else {
						fmt.Println("   The process exited cleanly but printed nothing — check runner_main.go for a silent early return or an empty rawCases/rawExpecteds payload.")
					}
				}

				// Safety check: harness self-reports EXPECTED_CASES / EXECUTED_CASES.
				// A silent parsing failure (0 cases run) would otherwise look like
				// "all tests passed" because no STATUS: FAILED line is ever printed.
				if strings.Contains(outputStr, "HARNESS_ERROR") {
					allPassed = false
					fmt.Println(lipgloss.NewStyle().Foreground(errorColor).Bold(true).
						Render("Harness reported an internal error:"))
					for _, line := range strings.Split(outputStr, "\n") {
						if strings.Contains(line, "HARNESS_ERROR") {
							fmt.Println("   " + line)
						}
					}
				}

				executed, execOk := extractIntAfterLabel(outputStr, "EXECUTED_CASES:")
				expected, expOk := extractIntAfterLabel(outputStr, "EXPECTED_CASES:")
				if !execOk || !expOk {
					allPassed = false
					fmt.Println(lipgloss.NewStyle().Foreground(errorColor).Bold(true).
						Render("Could not confirm test cases executed (missing EXECUTED_CASES/EXPECTED_CASES marker)."))
					// The markers being missing usually means the binary crashed
					// mid-run (e.g. a panic from the user's solution, or a slice-
					// bounds panic from the in-place []T[:k] truncation). Dump
					// whatever raw output DID come out — including any Go panic
					// stack trace — since that's the only way to see why.
					if strings.TrimSpace(outputStr) != "" {
						fmt.Println(lipgloss.NewStyle().Foreground(warningColor).Bold(true).
							Render("   Raw sandbox output:"))
						for _, line := range strings.Split(strings.TrimRight(outputStr, "\n"), "\n") {
							fmt.Println("   " + line)
						}
					}
				} else if executed != expected || executed != len(probData.Tests) {
					allPassed = false
					fmt.Println(lipgloss.NewStyle().Foreground(errorColor).Bold(true).Render(
						fmt.Sprintf("Only %d of %d test case(s) actually executed — check input parsing.", executed, len(probData.Tests)),
					))
				}

				for _, outcome := range runner.ParseLeetCodeHarnessOutput(outputStr) {
					if !outcome.Passed {
						allPassed = false
					}
					outcomes = append(outcomes, outcome)
				}
				os.Remove(binaryPath)

			} else {
				// Standard CF / AtCoder: execute each case, collect result.
				// Standard CF / AtCoder: execute each case, collect result.
				for i, testCase := range probData.Tests {
					res := runner.ExecuteTest(i+1, binaryPath, testCase, probData.TimeLimitMs, probData.MemoryLimitMb)
					if res.TimedOut || !res.Passed {
						allPassed = false
					}
					outcomes = append(outcomes, ui.CaseOutcome{
						CaseNum:  res.CaseNum,
						Passed:   res.Passed,
						TimedOut: res.TimedOut,
						Duration: res.Duration,
						Input:    res.Input,
						Got:      res.Got,
						Expected: res.Expected,
					})
				}
				os.Remove(binaryPath)
			}

			// On a clean pass, copy the submission-ready code to the
			// clipboard: the full file for CF/AtCoder, or the
			// package/import-stripped function body for LeetCode (which is
			// what LeetCode's own submission box expects). Decided BEFORE
			// rendering so the box can show it inline instead of a
			// separate line underneath.
			copied := false
			if allPassed {
				var codeToCopy string
				if probData.Platform == "leetcode" {
					codeToCopy = leetcodeCleanSource
				} else if raw, readErr := os.ReadFile(solutionFile); readErr == nil {
					codeToCopy = string(raw)
				}
				if codeToCopy != "" {
					copied = clipboard.WriteAll(codeToCopy) == nil
				}
			}

			// Render all cases + summary (with clipboard-copied notice, if
			// any) inside one bordered box.
			ui.RenderTestBlock(outcomes, allPassed, copied, probData.Platform)
		}

	ShowMenu:
		// Interact with selection control panel panel menu dashboard
		selection := renderAndListenMenu(menuItems, &menuCursorIndex)

		switch selection {
		case "run":
			continue
		case "add":
			addCustomTestCase(probData)
		case "remove":
			removeCustomTestCase(probData)
		case "runall":
			runAllCSESTests(probData, solutionFile)
		case "reload":
			fmt.Println("\n Fetching fresh problem payload from clipboard...")
			if loadFromClipboard() {
				fmt.Println(lipgloss.NewStyle().Foreground(successColor).Render(" ✓ Problem updated successfully!"))
			}
		case "exit":
			fmt.Println("\nExiting Competitive Gopher CLI. Happy Coding!")
			exitApp(0)
		}
	}
}

// addCustomTestCase prompts for a new test case. For LeetCode, it shows an
// existing case as a format example and REFUSES the new one outright if it
// doesn't parse against the function's actual parameters. For Codeforces/
// AtCoder it accepts free-form multi-line stdin/stdout blocks.
func addCustomTestCase(probData *parser.ProblemData) {
	reader := bufio.NewReader(os.Stdin)

	if probData.Platform == "leetcode" {
		if len(probData.Tests) > 0 {
			fmt.Println("\n Match this exact format (as scraped from LeetCode):")
			fmt.Printf("   input:    %s", probData.Tests[0].Input)
			if !strings.HasSuffix(probData.Tests[0].Input, "\n") {
				fmt.Println()
			}
			fmt.Printf("   expected: %s", probData.Tests[0].Expected)
			if !strings.HasSuffix(probData.Tests[0].Expected, "\n") {
				fmt.Println()
			}
		}

		details, err := runner.ParseLeetCodeSignature(probData.FunctionSignature)
		if err != nil {
			fmt.Println(" 🚨 Could not parse the function signature; can't validate a custom case.")
			pause(reader)
			return
		}

		fmt.Print("\n Input: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimRight(input, "\n") + "\n"

		if _, convErr := runner.ConvertInputToJSONMap(input, details.Params); convErr != nil {
			fmt.Printf(" 🚨 Rejected — input doesn't match the required format: %v\n", convErr)
			fmt.Println(" Keep the exact \"paramName = value, paramName2 = value2\" shape shown above.")
			pause(reader)
			return
		}

		fmt.Print(" Expected output: ")
		expected, _ := reader.ReadString('\n')
		expected = strings.TrimRight(expected, "\n") + "\n"

		probData.Tests = append(probData.Tests, parser.TestCase{Input: input, Expected: expected})
		fmt.Println(" ✓ Custom case added.")
		syncClipboard(probData)
		pause(reader)
		return
	}

	// Codeforces / AtCoder: raw multi-line stdin/stdout blocks.
	fmt.Println("\n Enter input (end with a line containing only ###):")
	input := readMultiline(reader)
	fmt.Println(" Enter expected output (end with a line containing only ###):")
	expected := readMultiline(reader)

	probData.Tests = append(probData.Tests, parser.TestCase{Input: input, Expected: expected})
	fmt.Println(" ✓ Custom case added.")
	syncClipboard(probData)
	pause(reader)
}

// removeCustomTestCase lists current cases and deletes the one the user picks.
func removeCustomTestCase(probData *parser.ProblemData) {
	reader := bufio.NewReader(os.Stdin)

	if len(probData.Tests) == 0 {
		fmt.Println("\n No test cases to remove.")
		pause(reader)
		return
	}

	fmt.Println("\n Current test cases:")
	for i, t := range probData.Tests {
		preview := strings.SplitN(strings.TrimSpace(t.Input), "\n", 2)[0]
		if len(preview) > 50 {
			preview = preview[:50] + "…"
		}
		fmt.Printf("   [%d] %s\n", i+1, preview)
	}
	fmt.Print("\n Enter the number to remove (0 to cancel): ")

	line, _ := reader.ReadString('\n')
	n, convErr := strconv.Atoi(strings.TrimSpace(line))
	if convErr != nil || n == 0 {
		fmt.Println(" Cancelled.")
		pause(reader)
		return
	}
	if n < 1 || n > len(probData.Tests) {
		fmt.Println(" 🚨 Invalid selection.")
		pause(reader)
		return
	}

	probData.Tests = append(probData.Tests[:n-1], probData.Tests[n:]...)
	fmt.Println(" ✓ Removed.")
	syncClipboard(probData)
	pause(reader)
}

// runAllCSESTests compiles the current solution once, then runs it against
// every N.in/N.out pair in the local CSES test bank for probData.ProblemID
// (see internal/runner/csesbank.go). Passing cases update a single live
// counter line in place; a failure freezes that line, prints a truncated
// diff, and pauses on a [c]ontinue / [o]pen in editor / [s]top prompt
// before moving on -- "open" writes the FULL untruncated case to a scratch
// file and shells out to Zed, then re-shows the same prompt.
func runAllCSESTests(probData *parser.ProblemData, solutionFile string) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n ⚙ Compiling for full test bank run...")
	binaryPath, err := runner.CompileSolution(solutionFile)
	if err != nil {
		fmt.Printf(" 🚨 Compile failed:\n%s\n", err.Error())
		pause(reader)
		return
	}
	defer os.Remove(binaryPath)

	tests, err := runner.LoadCSESTestBank(probData.ProblemID)
	if err != nil {
		fmt.Printf(" 🚨 %v\n", err)
		pause(reader)
		return
	}

	total := len(tests)
	acCount, waCount, tleCount := 0, 0, 0

	fmt.Println()
caseLoop:
	for i, tc := range tests {
		res := runner.ExecuteTest(i+1, binaryPath, tc, probData.TimeLimitMs, probData.MemoryLimitMb)

		if res.Passed {
			acCount++
			ui.RenderRunAllProgress(i+1, total, acCount, waCount, tleCount)
			continue
		}

		if res.TimedOut {
			tleCount++
		} else {
			waCount++
		}
		fmt.Println() // end the live counter line before printing the diff below it

		ui.RenderRunAllFailure(res.CaseNum, res.TimedOut, res.Input, res.Expected, res.Got)

		for {
			fmt.Print(" [c] continue   [o] open in Zed   [s] stop\n")
			switch readSingleKey() {
			case 'o', 'O':
				path, writeErr := writeFailureScratchFile(res.CaseNum, res.Input, res.Expected, res.Got)
				if writeErr != nil {
					fmt.Printf(" 🚨 Could not write scratch file: %v\n", writeErr)
					continue // retry the prompt, not the outer test loop
				}
				if startErr := exec.Command("zed", path).Start(); startErr != nil {
					fmt.Printf(" 🚨 Could not launch zed: %v\n", startErr)
				}
				// Loop again -- opening the file doesn't resolve continue/stop by itself.
			case 's', 'S':
				fmt.Printf("\n Stopped at %d/%d (%d AC, %d WA, %d TLE).\n", i+1, total, acCount, waCount, tleCount)
				pause(reader)
				return
			default: // 'c'/'C' or anything else — treat as continue
				fmt.Println()
				continue caseLoop
			}
		}
	}

	fmt.Println()
	ui.RenderRunAllSummary(acCount, total)
	pause(reader)
}

// writeFailureScratchFile writes the FULL (untruncated) input/expected/got
// for one failing case to a temp file, in the shape asked for: INPUT: /
// EXPECTED OUTPUT: / PROGRAM OUTPUT:, so it can be opened directly in an
// editor for a closer look.
func writeFailureScratchFile(caseNum int, input, expected, got string) (string, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("go-cp-cli-wa-case-%d.txt", caseNum))
	content := fmt.Sprintf("INPUT:\n%s\n\nEXPECTED OUTPUT:\n%s\n\nPROGRAM OUTPUT:\n%s\n", input, expected, got)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// readSingleKey reads one raw keypress without waiting for Enter. Used for
// the lightweight [c]/[o]/[s] prompt during Run All instead of the full
// arrow-key menu, so a failure doesn't force a full redraw to respond to.
func readSingleKey() byte {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return 's' // fail safe: stop rather than loop forever on a broken terminal
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	buf := make([]byte, 1)
	if _, err := os.Stdin.Read(buf); err != nil {
		return 's'
	}
	return buf[0]
}

// readMultiline reads lines until one contains only "###".
func readMultiline(reader *bufio.Reader) string {
	var sb strings.Builder
	for {
		line, _ := reader.ReadString('\n')
		trimmed := strings.TrimRight(line, "\n")
		if trimmed == "###" {
			break
		}
		sb.WriteString(trimmed)
		sb.WriteString("\n")
	}
	return sb.String()
}

// syncClipboard writes the current problem data back to the clipboard so
// "Reload Latest Problem from Clipboard" doesn't wipe out cases you just
// added/removed. Best-effort: failures are silently ignored.
func syncClipboard(probData *parser.ProblemData) {
	data, err := json.Marshal(probData)
	if err != nil {
		return
	}
	_ = clipboard.WriteAll(string(data))
}

// pause holds the screen so confirmation/error text from add/remove isn't
// instantly wiped by the next screen clear.
func pause(reader *bufio.Reader) {
	fmt.Print("\n Press Enter to continue...")
	reader.ReadString('\n')
}

// extractIntAfterLabel scans harness stdout for a line like "EXECUTED_CASES: 3"
// and returns the parsed integer, or ok=false if the label wasn't found/parseable.
func extractIntAfterLabel(output, label string) (int, bool) {
	_, after, ok := strings.Cut(output, label)
	if !ok {
		return 0, false
	}
	rest := strings.TrimSpace(after)
	// Take digits up to the next non-digit (newline, space, etc.)
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n := 0
	for _, c := range rest[:end] {
		n = n*10 + int(c-'0')
	}
	return n, true
}

// Intercepts Terminal Input Descriptor, handles drawing custom cursor, and
// catches raw arrow keys. The selected item is always highlighted with a
// fixed bright color (ui.MenuHighlightColor), not the platform's brand
// color -- AtCoder's brand is near-black, which made the selected item
// look dim/invisible on a dark terminal.
func renderAndListenMenu(items []MenuItem, cursorIndex *int) string {
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(ui.MenuHighlightColor).
		Bold(true).
		Padding(0, 1)

	unselectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A6E22E")).
		Padding(0, 1)

	for {
		ui.RenderMenuHint()

		for i, item := range items {
			if i == *cursorIndex {
				fmt.Printf("  ➤ %s\n", selectedStyle.Render(item.Label))
			} else {
				fmt.Printf("    %s\n", unselectedStyle.Render(item.Label))
			}
		}

		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("\nTerminal failure. Exiting...")
			exitApp(1)
		}

		buf := make([]byte, 3)
		numRead, readErr := os.Stdin.Read(buf)
		term.Restore(int(os.Stdin.Fd()), oldState)

		if readErr != nil {
			continue
		}

		if numRead == 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65: // Up Arrow
				if *cursorIndex > 0 {
					*cursorIndex--
				} else {
					*cursorIndex = len(items) - 1
				}
			case 66: // Down Arrow
				if *cursorIndex < len(items)-1 {
					*cursorIndex++
				} else {
					*cursorIndex = 0
				}
			}
		} else if numRead == 1 && (buf[0] == 13 || buf[0] == 10) {
			return items[*cursorIndex].Key
		} else if numRead == 1 && buf[0] == 3 {
			fmt.Println("\nProcess aborted.")
			exitApp(0)
		}

		fmt.Printf("\033[%dA", len(items)+2)
	}
}
