package exa

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/shopspring/decimal"
)

// Request defines the input for a batch calculation.
type Request struct {
	Inputs map[string]any `json:"inputs"`
	Policy []Calculation  `json:"policy"`
}

// Calculation defines a single formula with an identifier.
type Calculation struct {
	ID         string `json:"id"`
	Expression string `json:"expr"`
	
	// Internal AST storage
	ast *cel.Ast
}

// Result is the final output map of calculated values.
type Result map[string]decimal.Decimal

// Custom error definitions for programmatic handling.
var (
	ErrCircularDependency = fmt.Errorf("circular dependency detected")
	ErrDuplicateID        = fmt.Errorf("duplicate identifier found")
)

type EvalError struct {
	ID    string
	Inner error
}

func (e *EvalError) Error() string {
	return fmt.Sprintf("eval error for [%s]: %v", e.ID, e.Inner)
}

func (e *EvalError) Unwrap() error { return e.Inner }

// Compute is a convenience function for one-off calculations using a default engine.
func Compute(req Request) (Result, error) {
	return NewEngine().Compute(context.Background(), req)
}
