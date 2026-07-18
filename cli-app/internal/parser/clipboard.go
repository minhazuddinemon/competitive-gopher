package parser

// ProblemData maps incoming clipboard context across Codeforces, AtCoder, and LeetCode
type ProblemData struct {
	Platform          string     `json:"platform"`
	Title             string     `json:"title"`
	TimeLimitMs       int        `json:"time_limit_ms"`
	MemoryLimitMb     int        `json:"memory_limit_mb"`
	OrderMatters      bool       `json:"order_matters"`      // Captured globally across platforms
	FunctionSignature string     `json:"function_signature"` // LeetCode specific
	Tests             []TestCase `json:"tests"`
}

type TestCase struct {
	// For CF/AtCoder, Input is a raw line block string.
	// For LeetCode, Input can be stored as a flat string representation of its key-value arguments object.
	Input    string `json:"input"`
	Expected string `json:"expected"`
}
