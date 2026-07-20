package parser

import (
	"encoding/json"
	"errors"
)

// // TestCase represents an individual input/output pair scraped from the judge.
// type TestCase struct {
// 	Input    string `json:"input"`
// 	Expected string `json:"expected"`
// }

// // ProblemData represents the core JSON structure sent from the browser extension.
// type ProblemData struct {
// 	Platform          string     `json:"platform"`
// 	Title             string     `json:"title"`
// 	Type              string     `json:"type"`
// 	DataStructure     string     `json:"data_structure"`
// 	TimeLimitMs       int        `json:"time_limit_ms"`
// 	MemoryLimitMb     int        `json:"memory_limit_mb"`
// 	Tests             []TestCase `json:"tests"`
// 	FunctionSignature string     `json:"function_signature,omitempty"`
// }

// ParseClipboardJSON validates and converts the raw string payload into a usable Go struct.
func ParseClipboardJSON(rawJSON string) (*ProblemData, error) {
	if rawJSON == "" {
		return nil, errors.New("clipboard is empty")
	}

	var data ProblemData
	err := json.Unmarshal([]byte(rawJSON), &data)
	if err != nil {
		return nil, errors.New("clipboard content is not valid CP JSON format")
	}

	// Ensure we are filtering out unsupported platforms (Now including CSES)
	if data.Platform != "codeforces" && data.Platform != "atcoder" && data.Platform != "leetcode" && data.Platform != "cses" {
		return nil, errors.New("unsupported platform found on clipboard: " + data.Platform)
	}

	return &data, nil
}
