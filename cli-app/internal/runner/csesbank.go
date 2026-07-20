package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"go-cp-cli/cli-app/internal/parser"
)

// csesInFilePattern matches CSES-style test input filenames: "1.in",
// "12.in", etc. -- the numeric group is what we sort on, since a plain
// lexicographic directory listing would put "10.in" before "2.in".
var csesInFilePattern = regexp.MustCompile(`^(\d+)\.in$`)

// CSESTestBankDir returns the root directory local CSES test files are
// read from. Defaults to the user's fixed path; override with the
// CSES_TESTDATA_DIR environment variable if that ever needs to move.
func CSESTestBankDir() string {
	if dir := os.Getenv("CSES_TESTDATA_DIR"); dir != "" {
		return dir
	}
	return "/home/minhaz/cses_local_tests"
}

// HasCSESTestBank reports whether a local test folder exists for the given
// problem ID. Used to decide whether "Run All Test Cases" should even
// appear in the menu, so it silently doesn't show up for problems you
// haven't downloaded test data for yet, rather than erroring later.
func HasCSESTestBank(problemID string) bool {
	if problemID == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(CSESTestBankDir(), problemID))
	return err == nil && info.IsDir()
}

// LoadCSESTestBank reads every N.in/N.out pair under
// <CSESTestBankDir>/<problemID>/, sorted numerically by N (not
// lexicographically -- "2.in" must sort before "10.in", which a plain
// os.ReadDir listing would get wrong).
func LoadCSESTestBank(problemID string) ([]parser.TestCase, error) {
	if problemID == "" {
		return nil, fmt.Errorf("no CSES problem ID on this clipboard payload")
	}

	dir := filepath.Join(CSESTestBankDir(), problemID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read CSES test bank %s: %w", dir, err)
	}

	var nums []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := csesInFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, convErr := strconv.Atoi(m[1])
		if convErr != nil {
			continue
		}
		nums = append(nums, n)
	}
	sort.Ints(nums)

	if len(nums) == 0 {
		return nil, fmt.Errorf("no N.in test files found in %s", dir)
	}

	tests := make([]parser.TestCase, 0, len(nums))
	for _, n := range nums {
		inPath := filepath.Join(dir, fmt.Sprintf("%d.in", n))
		outPath := filepath.Join(dir, fmt.Sprintf("%d.out", n))

		inBytes, readErr := os.ReadFile(inPath)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read %s: %w", inPath, readErr)
		}
		outBytes, readErr := os.ReadFile(outPath)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read %s (input file exists but its matching .out is missing): %w", outPath, readErr)
		}

		tests = append(tests, parser.TestCase{
			Input:    string(inBytes),
			Expected: string(outBytes),
		})
	}

	return tests, nil
}
