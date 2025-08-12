package metrics

import (
	"fmt"
	"strconv"
	"strings"
)

// evaluateSimpleMath evaluates basic math expressions for metric calculations
// Supports: +, -, *, / and parentheses with variable substitution
// Optimised for common patterns like "value / 1000000" for unit conversion
func evaluateSimpleMath(expr string, variables map[string]float64) (float64, error) {
	// Replace variables with their values
	processed := expr
	for name, value := range variables {
		processed = strings.ReplaceAll(processed, name, fmt.Sprintf("%f", value))
	}

	// Handle common patterns directly for performance
	processed = strings.TrimSpace(processed)

	// Simple division pattern: "number / number"
	if parts := strings.Split(processed, "/"); len(parts) == 2 {
		left, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		right, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 == nil && err2 == nil && right != 0 {
			return left / right, nil
		}
	}

	// Simple multiplication pattern: "number * number"
	if parts := strings.Split(processed, "*"); len(parts) == 2 {
		left, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		right, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 == nil && err2 == nil {
			return left * right, nil
		}
	}

	// Simple addition pattern: "number + number"
	if parts := strings.Split(processed, "+"); len(parts) == 2 {
		left, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		right, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 == nil && err2 == nil {
			return left + right, nil
		}
	}

	// Simple subtraction pattern: "number - number"
	if parts := strings.Split(processed, "-"); len(parts) == 2 {
		left, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		right, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 == nil && err2 == nil {
			return left - right, nil
		}
	}

	// For more complex expressions with parentheses, use a simple recursive evaluator
	return evalExpression(processed)
}

// evalExpression handles more complex expressions with parentheses
func evalExpression(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)

	// Handle parentheses first
	expr, err := evaluateParentheses(expr)
	if err != nil {
		return 0, err
	}

	// Try to parse as a simple number first
	if val, err := strconv.ParseFloat(expr, 64); err == nil {
		return val, nil
	}

	// Handle operators in order of precedence
	return evaluateOperators(expr)
}

// evaluateParentheses resolves all parenthetical expressions
func evaluateParentheses(expr string) (string, error) {
	for strings.Contains(expr, "(") {
		start := strings.LastIndex(expr, "(")
		if start == -1 {
			break
		}
		end := strings.Index(expr[start:], ")")
		if end == -1 {
			return "", fmt.Errorf("unmatched parentheses")
		}
		end += start

		// Evaluate the expression inside parentheses
		inner := expr[start+1 : end]
		result, err := evalExpression(inner)
		if err != nil {
			return "", err
		}

		// Replace the parentheses expression with its result
		expr = expr[:start] + fmt.Sprintf("%f", result) + expr[end+1:]
	}
	return expr, nil
}

// evaluateOperators handles arithmetic operators in precedence order
func evaluateOperators(expr string) (float64, error) {
	// Handle division (higher precedence)
	if result, ok, err := evaluateBinaryOp(expr, "/", func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	}); ok {
		return result, err
	}

	// Handle multiplication
	if result, ok, err := evaluateBinaryOp(expr, "*", func(a, b float64) (float64, error) {
		return a * b, nil
	}); ok {
		return result, err
	}

	// Handle addition
	if result, ok, err := evaluateBinaryOp(expr, "+", func(a, b float64) (float64, error) {
		return a + b, nil
	}); ok {
		return result, err
	}

	// Handle subtraction
	if idx := strings.LastIndex(expr, "-"); idx > 0 { // > 0 to avoid negative numbers
		left, err1 := evalExpression(expr[:idx])
		right, err2 := evalExpression(expr[idx+1:])
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("invalid subtraction")
		}
		return left - right, nil
	}

	return 0, fmt.Errorf("unable to evaluate expression: %s", expr)
}

// evaluateBinaryOp evaluates a binary operation
func evaluateBinaryOp(expr, op string, operation func(float64, float64) (float64, error)) (float64, bool, error) {
	idx := strings.Index(expr, op)
	if idx == -1 {
		return 0, false, nil
	}

	left, err1 := evalExpression(expr[:idx])
	right, err2 := evalExpression(expr[idx+1:])
	if err1 != nil || err2 != nil {
		return 0, true, fmt.Errorf("invalid %s operation", op)
	}

	result, err := operation(left, right)
	return result, true, err
}
