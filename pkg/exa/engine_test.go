package exa

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestExa_FullFlow(t *testing.T) {
	engine := NewEngine()

	req := Request{
		Inputs: map[string]any{
			"base_pay": "5000000",
			"tax_rate": 0.1,
		},
		Policy: []Calculation{
			{ID: "tax_amount", Expression: "base_pay * tax_rate"},
			{ID: "final_pay", Expression: "base_pay - tax_amount"},
		},
	}

	res, err := engine.Compute(context.Background(), req)
	assert.NoError(t, err)

	// base_pay(5000000) * tax_rate(0.1) = 500000
	assert.True(t, res["tax_amount"].Equal(decimal.NewFromInt(500000)))
	// 5000000 - 500000 = 4500000
	assert.True(t, res["final_pay"].Equal(decimal.NewFromInt(4500000)))
}

func TestExa_AutomaticDependency(t *testing.T) {
	// Mixed order: final_pay depends on tax_amount which is defined AFTER it
	req := Request{
		Inputs: map[string]any{"base": 1000},
		Policy: []Calculation{
			{ID: "total", Expression: "base + tax"},
			{ID: "tax", Expression: "base * 0.1"},
		},
	}

	res, err := Compute(req)
	assert.NoError(t, err)

	assert.True(t, res["tax"].Equal(decimal.NewFromInt(100)))
	assert.True(t, res["total"].Equal(decimal.NewFromInt(1100)))
}

func TestExa_MixedTypes(t *testing.T) {
	req := Request{
		Inputs: map[string]any{
			"d": decimal.NewFromInt(10),
			"i": 5,
			"s": "2",
		},
		Policy: []Calculation{
			{ID: "res", Expression: "(d + i) * s"}, // (10 + 5) * 2 = 30
		},
	}

	res, err := Compute(req)
	assert.NoError(t, err)
	assert.True(t, res["res"].Equal(decimal.NewFromInt(30)))
}

func TestExa_Builtins(t *testing.T) {
	req := Request{
		Inputs: map[string]any{"vals": []any{10, "20.5", decimal.NewFromInt(30)}},
		Policy: []Calculation{
			{ID: "s", Expression: "sum(vals)"},
			{ID: "m", Expression: "max(10.5, 20)"},
		},
	}

	res, err := Compute(req)
	assert.NoError(t, err)
	assert.True(t, res["s"].Equal(decimal.RequireFromString("60.5")))
	assert.True(t, res["m"].Equal(decimal.RequireFromString("20")))
}
