package metrics

import (
	"fmt"
	"strconv"
	"strings"
)

// CompiledExpression represents a pre-compiled math expression
// that can be evaluated efficiently without string parsing
type CompiledExpression interface {
	Evaluate(variables map[string]float64) (float64, error)
}

// constantExpr is a compiled constant value
type constantExpr struct {
	value float64
}

func (c *constantExpr) Evaluate(_ map[string]float64) (float64, error) {
	return c.value, nil
}

// variableExpr is a compiled variable reference
type variableExpr struct {
	name string
}

func (v *variableExpr) Evaluate(variables map[string]float64) (float64, error) {
	if val, ok := variables[v.name]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("variable %s not found", v.name)
}

// binaryOpExpr is a compiled binary operation
type binaryOpExpr struct {
	left  CompiledExpression
	right CompiledExpression
	op    func(float64, float64) (float64, error)
}

func (b *binaryOpExpr) Evaluate(variables map[string]float64) (float64, error) {
	leftVal, err := b.left.Evaluate(variables)
	if err != nil {
		return 0, err
	}
	rightVal, err := b.right.Evaluate(variables)
	if err != nil {
		return 0, err
	}
	return b.op(leftVal, rightVal)
}

// CompileExpression compiles a math expression at startup for efficient evaluation
func CompileExpression(expr string) (CompiledExpression, error) {
	return compileExpr(strings.TrimSpace(expr))
}

func compileExpr(expr string) (CompiledExpression, error) {
	expr = strings.TrimSpace(expr)

	// Handle parentheses first
	if strings.Contains(expr, "(") {
		return compileWithParentheses(expr)
	}

	// Try to parse as a constant
	if val, err := strconv.ParseFloat(expr, 64); err == nil {
		return &constantExpr{value: val}, nil
	}

	// Check for operators (in reverse precedence order for parsing)
	// Addition/Subtraction (lowest precedence)
	if idx := findOperator(expr, "+", "-"); idx >= 0 {
		op := expr[idx]
		left, err := compileExpr(expr[:idx])
		if err != nil {
			return nil, err
		}
		right, err := compileExpr(expr[idx+1:])
		if err != nil {
			return nil, err
		}

		var opFunc func(float64, float64) (float64, error)
		if op == '+' {
			opFunc = func(a, b float64) (float64, error) { return a + b, nil }
		} else {
			opFunc = func(a, b float64) (float64, error) { return a - b, nil }
		}
		return &binaryOpExpr{left: left, right: right, op: opFunc}, nil
	}

	// Multiplication/Division (higher precedence)
	if idx := findOperator(expr, "*", "/"); idx >= 0 {
		op := expr[idx]
		left, err := compileExpr(expr[:idx])
		if err != nil {
			return nil, err
		}
		right, err := compileExpr(expr[idx+1:])
		if err != nil {
			return nil, err
		}

		var opFunc func(float64, float64) (float64, error)
		if op == '*' {
			opFunc = func(a, b float64) (float64, error) { return a * b, nil }
		} else {
			opFunc = func(a, b float64) (float64, error) {
				if b == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				return a / b, nil
			}
		}
		return &binaryOpExpr{left: left, right: right, op: opFunc}, nil
	}

	// Must be a variable
	return &variableExpr{name: expr}, nil
}

func compileWithParentheses(expr string) (CompiledExpression, error) {
	// Find innermost parentheses
	start := strings.LastIndex(expr, "(")
	if start == -1 {
		return compileExpr(expr)
	}

	end := strings.Index(expr[start:], ")")
	if end == -1 {
		return nil, fmt.Errorf("unmatched parentheses")
	}
	end += start

	// Compile the expression inside parentheses
	inner, err := compileExpr(expr[start+1 : end])
	if err != nil {
		return nil, err
	}

	// If the entire expression is just parentheses, return the inner expression
	if start == 0 && end == len(expr)-1 {
		return inner, nil
	}

	// Handle expressions with parentheses in the middle
	// We'll create a temporary variable to represent the parentheses result
	// and compile the full expression
	before := expr[:start]
	after := expr[end+1:]

	// If there's nothing before and after starts with an operator,
	// compile as binary operation
	after = strings.TrimSpace(after)
	before = strings.TrimSpace(before)

	if before == "" && len(after) > 0 {
		// Pattern like: (expr) / value
		op := after[0]
		if op == '/' || op == '*' || op == '+' || op == '-' {
			rightExpr, err := compileExpr(after[1:])
			if err != nil {
				return nil, err
			}

			var opFunc func(float64, float64) (float64, error)
			switch op {
			case '/':
				opFunc = func(a, b float64) (float64, error) {
					if b == 0 {
						return 0, fmt.Errorf("division by zero")
					}
					return a / b, nil
				}
			case '*':
				opFunc = func(a, b float64) (float64, error) { return a * b, nil }
			case '+':
				opFunc = func(a, b float64) (float64, error) { return a + b, nil }
			case '-':
				opFunc = func(a, b float64) (float64, error) { return a - b, nil }
			}

			return &binaryOpExpr{left: inner, right: rightExpr, op: opFunc}, nil
		}
	} else if after == "" && len(before) > 0 {
		// Pattern like: value / (expr)
		// Find the operator before the parentheses
		for i := len(before) - 1; i >= 0; i-- {
			if before[i] == '/' || before[i] == '*' || before[i] == '+' || before[i] == '-' {
				leftExpr, err := compileExpr(before[:i])
				if err != nil {
					return nil, err
				}

				op := before[i]
				var opFunc func(float64, float64) (float64, error)
				switch op {
				case '/':
					opFunc = func(a, b float64) (float64, error) {
						if b == 0 {
							return 0, fmt.Errorf("division by zero")
						}
						return a / b, nil
					}
				case '*':
					opFunc = func(a, b float64) (float64, error) { return a * b, nil }
				case '+':
					opFunc = func(a, b float64) (float64, error) { return a + b, nil }
				case '-':
					opFunc = func(a, b float64) (float64, error) { return a - b, nil }
				}

				return &binaryOpExpr{left: leftExpr, right: inner, op: opFunc}, nil
			}
		}
	}

	// If we couldn't handle it simply, recursively compile the full expression
	// This is less efficient but maintains correctness
	return nil, fmt.Errorf("complex parentheses pattern not supported: %s", expr)
}

// findOperator finds the rightmost occurrence of any operator
// (for left-to-right evaluation)
func findOperator(expr string, ops ...string) int {
	lastIdx := -1
	for _, op := range ops {
		if idx := strings.LastIndex(expr, op); idx > lastIdx && idx > 0 {
			// Check it's not a negative number
			if op == "-" && idx > 0 {
				prevChar := expr[idx-1]
				if prevChar == '*' || prevChar == '/' || prevChar == '+' || prevChar == '-' {
					continue
				}
			}
			lastIdx = idx
		}
	}
	return lastIdx
}
