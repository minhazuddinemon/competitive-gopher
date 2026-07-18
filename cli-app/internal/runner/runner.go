package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go-cp-cli/cli-app/internal/parser"
)

// RunResult holds the tactical output data of a single test case run.
type RunResult struct {
	CaseNum  int
	Passed   bool
	TimedOut bool
	Duration time.Duration
	Input    string
	Got      string
	Expected string
}

// CompileSolution compiles the target go file into a temporary executable binary.
func CompileSolution(sourceFile string) (string, error) {
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		return "", fmt.Errorf("source file %s does not exist", sourceFile)
	}

	// Create a unique temporary binary name in the same directory
	binaryName := "./temp_solution_bin"

	// Execute: go build -o ./temp_solution_bin solution.go
	cmd := exec.Command("go", "build", "-o", binaryName, sourceFile)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("compilation failed:\n%s", stderr.String())
	}

	// Get full path to make executing it highly reliable
	absPath, err := filepath.Abs(binaryName)
	if err != nil {
		return binaryName, nil
	}
	return absPath, nil
}

// ExecuteTest runs the compiled binary against a single input string with a strict timeout.
func ExecuteTest(caseNum int, binaryPath string, test parser.TestCase, timeoutMs int) RunResult {
	result := RunResult{
		CaseNum:  caseNum,
		Input:    test.Input,
		Expected: test.Expected,
	}

	// Enforce strict TLE limit using Go Context Timeouts
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath)

	// Set up pipes for standard input/output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = bytes.NewBufferString(test.Input)

	startTime := time.Now()
	err := cmd.Run()
	result.Duration = time.Since(startTime)

	// Catch Time Limit Exceeded (TLE)
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Passed = false
		return result
	}

	if err != nil {
		result.Passed = false
		result.Got = "RUNTIME ERROR:\n" + stderr.String()
		return result
	}

	// Clean and normalize trailing lines/spaces for evaluation
	gotStr := normalizeOutput(stdout.String())
	expectedStr := normalizeOutput(test.Expected)

	result.Got = gotStr
	result.Passed = (gotStr == expectedStr)

	return result
}

// normalizeOutput strips weird carriage returns and trims excessive whitespace blocks
func normalizeOutput(s string) string {
	s = os.ExpandEnv(s) // clear variables if any
	lines := bytes.Split([]byte(s), []byte("\n"))
	var cleanedLines [][]byte

	for _, line := range lines {
		cleanedLines = append(cleanedLines, bytes.TrimSpace(line))
	}

	return string(bytes.TrimSpace(bytes.Join(cleanedLines, []byte("\n"))))
}
