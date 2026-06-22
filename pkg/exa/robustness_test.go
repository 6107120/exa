package exa

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestExa_EdgeCases(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine()

	t.Run("DivisionByZero", func(t *testing.T) {
		req := Request{
			Inputs: map[string]any{"a": 10, "b": 0},
			Policy: []Calculation{{ID: "res", Expression: "a / b"}},
		}
		_, err := engine.Compute(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "division by zero")
	})

	t.Run("CircularDependency", func(t *testing.T) {
		req := Request{
			Policy: []Calculation{
				{ID: "a", Expression: "b + 1"},
				{ID: "b", Expression: "a + 1"},
			},
		}
		_, err := engine.Compute(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular dependency")
	})

	t.Run("MissingInput", func(t *testing.T) {
		req := Request{
			Policy: []Calculation{{ID: "res", Expression: "unknown_var + 1"}},
		}
		_, err := engine.Compute(ctx, req)
		assert.Error(t, err)
		// CEL v0.28.0 returns unspecified type error when a variable is not found during check if not declared
		assert.Contains(t, err.Error(), "unexpected unspecified type")
	})

	t.Run("DuplicateIdentifiers", func(t *testing.T) {
		req := Request{
			Inputs: map[string]any{"a": 1},
			Policy: []Calculation{{ID: "a", Expression: "10"}},
		}
		_, err := engine.Compute(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate identifier")
	})

	t.Run("EmptyPolicy", func(t *testing.T) {
		req := Request{
			Inputs: map[string]any{"a": 1},
			Policy: []Calculation{},
		}
		res, err := engine.Compute(ctx, req)
		assert.NoError(t, err)
		assert.Empty(t, res)
	})
}

func TestExa_Robustness_Complex(t *testing.T) {
	t.Run("DeepChain", func(t *testing.T) {
		policy := []Calculation{
			{ID: "n1", Expression: "1"},
			{ID: "n2", Expression: "n1 + 1"},
			{ID: "n3", Expression: "n2 + 1"},
		}
		res, err := Compute(Request{Policy: policy})
		assert.NoError(t, err)
		assert.True(t, res["n3"].Equal(decimal.NewFromInt(3)))
	})

	t.Run("PrecisionCheck", func(t *testing.T) {
		req := Request{
			Policy: []Calculation{
				{ID: "p1", Expression: "0.1 + 0.2"},
				{ID: "p2", Expression: "1 / 3"}, // 0.3333...
				{ID: "p3", Expression: "p2 * 3"}, // Should be 1.0000... if high precision is kept
			},
		}
		res, err := Compute(req)
		assert.NoError(t, err)
		assert.True(t, res["p1"].Equal(decimal.RequireFromString("0.3")))
		// shopspring/decimal's default division precision is 16.
		// p2 will be 0.3333333333333333
		// p3 will be 0.9999999999999999
		// To get exactly 1, we'd need rational numbers, but for Decimal this is expected.
		assert.True(t, res["p3"].GreaterThan(decimal.RequireFromString("0.9999")))
	})

	t.Run("ListAndSumMixed", func(t *testing.T) {
		req := Request{
			Inputs: map[string]any{
				"items": []any{"10.5", 20, decimal.NewFromInt(30)},
			},
			Policy: []Calculation{{ID: "total", Expression: "sum(items)"}},
		}
		res, err := Compute(req)
		assert.NoError(t, err)
		assert.True(t, res["total"].Equal(decimal.RequireFromString("60.5")))
	})
	
	t.Run("Precision_DirtyFloatVsCleanString", func(t *testing.T) {
		// 1. float64 input: 0.1 in float64 is not exactly 0.1
		// decimal.NewFromFloat handles this by getting the shortest precise string,
		// but 1.0/3.0 passed as float64 is limited by 53-bit mantissa.
		fInput := 1.0 / 3.0
		sInput := "0.333333333333333333333333333333333333" // Ultra high precision string

		req := Request{
			Inputs: map[string]any{
				"f_val": fInput,
				"s_val": sInput,
			},
			Policy: []Calculation{
				{ID: "f_res", Expression: "f_val * 3"},
				{ID: "s_res", Expression: "s_val * 3"},
			},
		}
		res, err := Compute(req)
		assert.NoError(t, err)

		// float64 based result will be limited to ~16 digits
		assert.NotEqual(t, "1", res["f_res"].String())
		// string based result preserves the extreme precision
		assert.Equal(t, "0.999999999999999999999999999999999999", res["s_res"].String())
	})

	t.Run("AssertionFailure", func(t *testing.T) {
		req := Request{
			Inputs: map[string]any{"age": 15},
			Policy: []Calculation{{ID: "check", Expression: "assert(age >= 18, 'Underage access')"}},
		}
		_, err := Compute(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "assertion failed: Underage access")
	})
}
