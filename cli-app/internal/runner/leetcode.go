package runner

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PrepareLeetCodeSandbox coordinates parsing your solution, formatting the
// harness, and writing it out. It returns (sandboxDir, cleanedSource, err)
// -- cleanedSource is your solution with package/import/main stripped,
// exactly the shape LeetCode's submission box expects, so callers can
// clipboard-copy it on an all-pass run without re-deriving it.
func PrepareLeetCodeSandbox(userSourcePath string, inputJSON string, expectedJSON string, orderMatters bool, details *LeetCodeFuncDetails, processedCases []string, expecteds []string) (string, string, error) {
	// 1. Create a stable, hidden sandbox path in the system cache/temp directory
	sandboxDir := filepath.Join(os.TempDir(), "go-cp-cli-sandbox")
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	// 2. Parse your solution file into an AST
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, userSourcePath, nil, parser.ParseComments)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse your solution file: %w", err)
	}

	// 3. Surgically remove 'func main()' AND your own import block. Imports
	// are dropped (not merged) because goimports (step 9) adds back
	// whatever the spliced code actually needs against the harness's own
	// fixed imports, and removes anything left unused -- trying to hand-merge
	// two import lists here is what caused "X redeclared in this block".
	var cleanDecls []ast.Decl
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == "main" {
				continue
			}
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
		}
		cleanDecls = append(cleanDecls, decl)
	}
	node.Decls = cleanDecls

	// Print only the inner declarations, bypassing the file node, so we
	// don't duplicate the "package main" header line.
	var userCodeBuilder strings.Builder
	for _, decl := range node.Decls {
		if err := printer.Fprint(&userCodeBuilder, fset, decl); err != nil {
			return "", "", fmt.Errorf("failed to regenerate user code declaration: %w", err)
		}
		userCodeBuilder.WriteString("\n\n")
	}
	userCodeString := userCodeBuilder.String()

	// 5. Build dynamic programmatic variables based on the signature details
	var varDeclarations strings.Builder
	var unmarshalBlocks strings.Builder
	var functionArgs strings.Builder

	for _, param := range details.Params {
		fmt.Fprintf(&varDeclarations, "\t\tvar %s %s\n", param.Name, param.Type)
		fmt.Fprintf(&unmarshalBlocks, "\t\tunmarshalFlexible(caseData[%q], &%s)\n", param.Name, param.Name)

		if functionArgs.Len() > 0 {
			functionArgs.WriteString(", ")
		}
		functionArgs.WriteString(param.Name)
	}

	// 6. Get the raw template structure
	harnessTemplate := GetLeetCodeHarnessTemplate()

	// 6.5. Populate the harness template string with all variables. Case
	// and expected arrays are joined as raw JSON array literals (not
	// double-JSON-encoded) so the harness can unmarshal them directly.
	finalCode := fmt.Sprintf(harnessTemplate,
		userCodeString,
		serializeList(processedCases),
		serializeList(expecteds),
		orderMatters,
		len(processedCases), // expectedCaseCount — lets the harness self-report if it silently ran 0 cases
		varDeclarations.String(),
		unmarshalBlocks.String(),
		details.Returns[0],
		details.Name,
		functionArgs.String(),
	)

	// 7. Write the unified file directly into the hidden folder
	sandboxFile := filepath.Join(sandboxDir, "runner_main.go")
	if err := os.WriteFile(sandboxFile, []byte(finalCode), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write sandbox runner source: %w", err)
	}

	// 8. Initialize a temporary go.mod inside the sandbox if it doesn't exist
	goModPath := filepath.Join(sandboxDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		cmd := exec.Command("go", "mod", "init", "leetcode/sandbox")
		cmd.Dir = sandboxDir
		_ = cmd.Run()
	}

	// 9. Reconcile imports: goimports adds whatever the spliced-in solution
	// code needs beyond the harness's own fixed imports, and drops anything
	// left unused (e.g. if the solution didn't actually need "sort").
	// Falls back to plain "go fmt" (and the old duplicate-import failure
	// mode) if goimports isn't installed/found on PATH.
	if _, lookErr := exec.LookPath("goimports"); lookErr == nil {
		cmdImports := exec.Command("goimports", "-w", "runner_main.go")
		cmdImports.Dir = sandboxDir
		_ = cmdImports.Run()
	} else {
		cmdFmt := exec.Command("go", "fmt", "runner_main.go")
		cmdFmt.Dir = sandboxDir
		_ = cmdFmt.Run()
	}

	return sandboxDir, userCodeString, nil
}

// CompileLeetCodeSandbox runs the compiler inside the hidden workspace using "."
func CompileLeetCodeSandbox(sandboxDir string) (string, error) {
	binaryName := "leetcode_worker"
	binaryPath := filepath.Join(sandboxDir, binaryName)

	os.Remove(binaryPath)

	// Compiling "." tells Go to look at the whole package folder, finding everything perfectly
	cmd := exec.Command("go", "build", "-o", binaryName, ".")
	cmd.Dir = sandboxDir

	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("compile failure in sandbox:\n%s", string(output))
	}

	return binaryPath, nil
}

// Helper function that just returns the raw string with %s formatting verbs
func GetLeetCodeHarnessTemplate() string {
	return `package main

import (
	"encoding/json"
	"fmt"
	// "os"
	"reflect"
	"strings"
	"sort"
)

// --- START USER CODE ---
%s
// --- END USER CODE ---

func main() {
	rawCases := %q
	rawExpecteds := %q
	orderMatters := %t
	expectedCaseCount := %d

	var cases []map[string]json.RawMessage
	var expecteds []json.RawMessage

	if err := json.Unmarshal([]byte(rawCases), &cases); err != nil {
		fmt.Printf("HARNESS_ERROR: failed to unmarshal test cases: %%v\n", err)
	}
	if err := json.Unmarshal([]byte(rawExpecteds), &expecteds); err != nil {
		fmt.Printf("HARNESS_ERROR: failed to unmarshal expected outputs: %%v\n", err)
	}

	fmt.Printf("EXPECTED_CASES: %%d\n", expectedCaseCount)

	executedCount := 0
	for i := 0; i < len(cases); i++ {
		caseData := cases[i]
		fmt.Printf("--- CASE %%d START ---\n", i+1)
		inputJSON, _ := json.Marshal(caseData)
		fmt.Printf("INPUT: %%s\n", string(inputJSON))

%s
%s
		var expected %s
		_ = unmarshalFlexible(expecteds[i], &expected)

		// Execute User Code
		got := %s(%s)

		if !orderMatters {
			normalizeOrdering(reflect.ValueOf(got))
			normalizeOrdering(reflect.ValueOf(expected))
		}

		if deepCompare(reflect.ValueOf(got), reflect.ValueOf(expected)) {
			fmt.Println("STATUS: PASSED")
		} else {
			fmt.Println("STATUS: FAILED")
			gotBytes, _ := json.Marshal(got)
			expBytes, _ := json.Marshal(expected)
			fmt.Printf("GOT: %%s\n", string(gotBytes))
			fmt.Printf("EXPECTED: %%s\n", string(expBytes))
		}
		executedCount++
	}

	fmt.Printf("EXECUTED_CASES: %%d\n", executedCount)
}

func deepCompare(v1, v2 reflect.Value) bool {
	if v1.Kind() == reflect.Ptr {
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return deepCompare(v1.Elem(), v2.Elem())
	}
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}

	// LeetCode judges treat a nil slice and an empty slice as the same
	// answer (e.g. "no triplets found" -> nil vs []). reflect.DeepEqual
	// does not, so slices are compared by length + element-wise recursion
	// instead, which also fixes the same nil-vs-empty issue at any
	// nesting depth (e.g. [][]int).
	if v1.Kind() == reflect.Slice {
		if v1.Len() != v2.Len() {
			return false
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepCompare(v1.Index(i), v2.Index(i)) {
				return false
			}
		}
		return true
	}

	return reflect.DeepEqual(v1.Interface(), v2.Interface())
}

func normalizeOrdering(v reflect.Value) {
	if !v.IsValid() || v.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < v.Len(); i++ {
		normalizeOrdering(v.Index(i))
	}
	if v.Len() <= 1 {
		return
	}
	switch v.Type().Elem().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		sort.Slice(v.Interface(), func(i, j int) bool { return v.Index(i).Int() < v.Index(j).Int() })
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		sort.Slice(v.Interface(), func(i, j int) bool { return v.Index(i).Uint() < v.Index(j).Uint() })
	case reflect.Float32, reflect.Float64:
		sort.Slice(v.Interface(), func(i, j int) bool { return v.Index(i).Float() < v.Index(j).Float() })
	case reflect.String:
		sort.Slice(v.Interface(), func(i, j int) bool { return v.Index(i).String() < v.Index(j).String() })
	case reflect.Slice:
		sort.Slice(v.Interface(), func(i, j int) bool {
			s1, s2 := v.Index(i), v.Index(j)
			minLen := s1.Len()
			if s2.Len() < minLen { minLen = s2.Len() }
			for k := 0; k < minLen; k++ {
				e1, e2 := s1.Index(k), s2.Index(k)
				if e1.Kind() == reflect.Int && e1.Int() != e2.Int() { return e1.Int() < e2.Int() }
				if e1.Kind() == reflect.String && e1.String() != e2.String() { return e1.String() < e2.String() }
			}
			return s1.Len() < s2.Len()
		})
	}
}

func unmarshalFlexible(data json.RawMessage, v interface{}) error {
	// []byte / [][]byte need special handling: LeetCode ships these as
	// arrays of single-character strings (e.g. ["5","3","."]), not base64,
	// which is what encoding/json normally expects for a []byte target.
	if handled, err := tryUnmarshalByteSlice(data, v); handled {
		return err
	}

	// 1. Try direct native unmarshal first
	if err := json.Unmarshal(data, v); err == nil {
		return nil
	}

	// 2. Fallback: If it's wrapped as a raw scraped text string, unpack it
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		str = strings.TrimSpace(str)
		// Strip assignment prefixes like "x = 121" -> "121"
		if idx := strings.Index(str, "="); idx != -1 {
			str = strings.TrimSpace(str[idx+1:])
		}
		// Try unmarshaling the clean inner string token directly into the target type
		return json.Unmarshal([]byte(str), v)
	}
	return fmt.Errorf("failed to parse unstructured input")
}

// tryUnmarshalByteSlice converts a JSON array of single-char strings into
// []byte or [][]byte. Returns handled=false for any other target type so
// the caller falls through to normal unmarshal logic.
func tryUnmarshalByteSlice(data json.RawMessage, v interface{}) (handled bool, err error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return false, nil
	}
	elemType := rv.Elem().Type()

	switch elemType {
	case reflect.TypeOf([]byte(nil)):
		var strs []string
		if err := json.Unmarshal(data, &strs); err != nil {
			return true, err
		}
		out := make([]byte, len(strs))
		for i, s := range strs {
			if len(s) > 0 {
				out[i] = s[0]
			}
		}
		rv.Elem().Set(reflect.ValueOf(out))
		return true, nil

	case reflect.TypeOf([][]byte(nil)):
		var strs [][]string
		if err := json.Unmarshal(data, &strs); err != nil {
			return true, err
		}
		out := make([][]byte, len(strs))
		for i, row := range strs {
			r := make([]byte, len(row))
			for j, s := range row {
				if len(s) > 0 {
					r[j] = s[0]
				}
			}
			out[i] = r
		}
		rv.Elem().Set(reflect.ValueOf(out))
		return true, nil
	}
	return false, nil
}
`
}

func serializeList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	return "[" + strings.Join(items, ",") + "]"
}
