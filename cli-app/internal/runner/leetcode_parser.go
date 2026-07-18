package runner

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

type ParamInfo struct {
	Name string
	Type string // e.g., "[]int", "int", "string"
}

type LeetCodeFuncDetails struct {
	Name    string
	Params  []ParamInfo
	Returns []string
}

// ParseLeetCodeSignature uses the Go AST to fully dissect the signature string
func ParseLeetCodeSignature(sig string) (*LeetCodeFuncDetails, error) {
	// Normalize spacing and append empty braces to make it a valid declaration
	cleanSig := strings.TrimSpace(sig)
	virtualSource := fmt.Sprintf("package main\n%s {}", cleanSig)

	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, "", virtualSource, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse function signature: %w", err)
	}

	for _, decl := range fileNode.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			details := &LeetCodeFuncDetails{
				Name: fn.Name.Name,
			}

			// Extract Input Parameters
			if fn.Type.Params != nil {
				for _, field := range fn.Type.Params.List {
					// Extract the type string directly from our source string
					typeStart := field.Type.Pos() - 1
					typeEnd := field.Type.End() - 1
					typeStr := virtualSource[typeStart:typeEnd]

					// Handle grouped parameters like (a, b int)
					for _, name := range field.Names {
						details.Params = append(details.Params, ParamInfo{
							Name: name.Name,
							Type: typeStr,
						})
					}
				}
			}

			// Extract Return Types
			if fn.Type.Results != nil {
				for _, field := range fn.Type.Results.List {
					typeStart := field.Type.Pos() - 1
					typeEnd := field.Type.End() - 1
					typeStr := virtualSource[typeStart:typeEnd]

					if len(field.Names) == 0 {
						details.Returns = append(details.Returns, typeStr)
					} else {
						for range field.Names {
							details.Returns = append(details.Returns, typeStr)
						}
					}
				}
			}

			return details, nil
		}
	}

	return nil, fmt.Errorf("no valid function signature found in input data")
}

// ConvertInputToJSONMap converts "param1 = val1, param2 = val2" into a JSON object string
func ConvertInputToJSONMap(rawInput string, params []ParamInfo) (string, error) {
	cleaned := strings.TrimSpace(rawInput)

	// We build a key-value structure mapping param name to its raw value string
	paramValues := make(map[string]string)

	for i, currentParam := range params {
		prefix := currentParam.Name + " ="
		idx := strings.Index(cleaned, prefix)
		if idx == -1 {
			// Fallback check if spaces are missing around assignment operator
			prefix = currentParam.Name + "="
			idx = strings.Index(cleaned, prefix)
			if idx == -1 {
				return "", fmt.Errorf("missing parameter %s in input data", currentParam.Name)
			}
		}

		valueStart := idx + len(prefix)
		valueEnd := len(cleaned)

		// If there is a next parameter, the current parameter value ends right before it
		if i+1 < len(params) {
			nextParam := params[i+1]
			nextPrefix := nextParam.Name + " ="
			nextIdx := strings.Index(cleaned, nextPrefix)
			if nextIdx == -1 {
				nextPrefix = nextParam.Name + "="
				nextIdx = strings.Index(cleaned, nextPrefix)
			}
			if nextIdx != -1 && nextIdx > valueStart {
				valueEnd = nextIdx
			}
		}

		// Extract the raw value string and strip trailing commas/whitespace
		valStr := strings.TrimSpace(cleaned[valueStart:valueEnd])
		if before, ok := strings.CutSuffix(valStr, ","); ok {
			valStr = before
		}
		paramValues[currentParam.Name] = strings.TrimSpace(valStr)
	}

	// Marshall the map into a structured JSON object string
	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for k, v := range paramValues {
		if !first {
			sb.WriteString(",")
		}
		first = false
		// Keys must be strings, values are preserved as raw JSON tokens
		fmt.Fprintf(&sb, "%q:%s", k, v)
	}
	sb.WriteString("}")

	return sb.String(), nil
}
