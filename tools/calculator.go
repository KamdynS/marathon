package tools

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// CalculatorTool evaluates a simple arithmetic expression using Go parser.
type CalculatorTool struct{}

func (c *CalculatorTool) Name() string { return "calculator" }
func (c *CalculatorTool) Description() string {
	return "Evaluate simple arithmetic expressions (e.g., 2+2*3)."
}
func (c *CalculatorTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": "arithmetic expression"}
}

func (c *CalculatorTool) Execute(ctx context.Context, input string) (string, error) {
	// Very minimal expression evaluator via AST for + - * / and parentheses.
	fs := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fs, "expr", input, 0)
	if err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}
	val, err := eval(expr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", val), nil
}

func eval(e ast.Expr) (float64, error) {
	switch v := e.(type) {
	case *ast.BasicLit:
		var out float64
		_, err := fmt.Sscanf(v.Value, "%f", &out)
		if err != nil {
			// try integers
			var i int64
			if _, err2 := fmt.Sscanf(v.Value, "%d", &i); err2 == nil {
				return float64(i), nil
			}
			return 0, fmt.Errorf("unsupported literal: %s", v.Value)
		}
		return out, nil
	case *ast.ParenExpr:
		return eval(v.X)
	case *ast.BinaryExpr:
		left, err := eval(v.X)
		if err != nil {
			return 0, err
		}
		right, err := eval(v.Y)
		if err != nil {
			return 0, err
		}
		switch v.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return left / right, nil
		default:
			return 0, fmt.Errorf("unsupported operator: %s", v.Op)
		}
	default:
		return 0, fmt.Errorf("unsupported expression: %T", e)
	}
}
