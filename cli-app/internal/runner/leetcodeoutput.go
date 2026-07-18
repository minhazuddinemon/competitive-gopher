package runner

import (
	"bufio"
	"strconv"
	"strings"

	"go-cp-cli/cli-app/internal/ui"
)

// ParseLeetCodeHarnessOutput turns the sandboxed LeetCode harness's raw
// stdout (the "--- CASE N START ---" / "INPUT:" / "STATUS:" / "GOT:" /
// "EXPECTED:" markers printed by the generated runner_main.go) into the
// same ui.CaseOutcome shape the Codeforces/AtCoder runner produces, so
// every platform renders through one consistent UI component.
func ParseLeetCodeHarnessOutput(output string) []ui.CaseOutcome {
	var results []ui.CaseOutcome
	scanner := bufio.NewScanner(strings.NewReader(output))

	currentCase := 0
	currentInput := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch {
		case strings.HasPrefix(line, "--- CASE") && strings.HasSuffix(line, "START ---"):
			// Line shape: "--- CASE 2 START ---"
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				if n, err := strconv.Atoi(fields[2]); err == nil {
					currentCase = n
				}
			}
			currentInput = ""

		case strings.HasPrefix(line, "INPUT: "):
			currentInput = strings.TrimPrefix(line, "INPUT: ")

		case line == "STATUS: PASSED":
			results = append(results, ui.CaseOutcome{
				CaseNum: currentCase,
				Passed:  true,
				Input:   currentInput,
			})

		case line == "STATUS: FAILED":
			got, expected := "", ""
			// The harness always prints GOT: then EXPECTED: as the next
			// two lines immediately after STATUS: FAILED.
			if scanner.Scan() {
				got = strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "GOT: ")
			}
			if scanner.Scan() {
				expected = strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "EXPECTED: ")
			}
			results = append(results, ui.CaseOutcome{
				CaseNum:  currentCase,
				Passed:   false,
				Input:    currentInput,
				Got:      got,
				Expected: expected,
			})
		}
	}
	return results
}
