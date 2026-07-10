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
	assert.True(t, res.Decimals["tax_amount"].Equal(decimal.NewFromInt(500000)))
	// 5000000 - 500000 = 4500000
	assert.True(t, res.Decimals["final_pay"].Equal(decimal.NewFromInt(4500000)))
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

	assert.True(t, res.Decimals["tax"].Equal(decimal.NewFromInt(100)))
	assert.True(t, res.Decimals["total"].Equal(decimal.NewFromInt(1100)))
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
	assert.True(t, res.Decimals["res"].Equal(decimal.NewFromInt(30)))
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
	assert.True(t, res.Decimals["s"].Equal(decimal.RequireFromString("60.5")))
	assert.True(t, res.Decimals["m"].Equal(decimal.RequireFromString("20")))
}

func TestExa_RoundDown(t *testing.T) {
	req := Request{
		Policy: []Calculation{
			{ID: "pos_digits", Expression: "round_down(157.89, 1)"},      // 157.8
			{ID: "zero_digits", Expression: "round_down(157.89, 0)"},     // 157
			{ID: "neg_tens", Expression: "round_down(157.89, -1)"},       // 150
			{ID: "neg_hundreds", Expression: "round_down(12345.678, -2)"}, // 12300
			{ID: "neg_value", Expression: "round_down(-157.89, -1)"},     // -150 (toward zero)
			{ID: "small_value", Expression: "round_down(7, -1)"},         // 0
		},
	}

	res, err := Compute(req)
	assert.NoError(t, err)
	assert.True(t, res.Decimals["pos_digits"].Equal(decimal.RequireFromString("157.8")))
	assert.True(t, res.Decimals["zero_digits"].Equal(decimal.NewFromInt(157)))
	assert.True(t, res.Decimals["neg_tens"].Equal(decimal.NewFromInt(150)))
	assert.True(t, res.Decimals["neg_hundreds"].Equal(decimal.NewFromInt(12300)))
	assert.True(t, res.Decimals["neg_value"].Equal(decimal.NewFromInt(-150)))
	assert.True(t, res.Decimals["small_value"].Equal(decimal.NewFromInt(0)))
}

func TestExa_VectorOperations(t *testing.T) {
	req := Request{
		Inputs: map[string]any{
			"salaries": []any{2000, 3000, 4000},
			"bonus_rate": 0.1,
			"fixed_bonus": []any{100, 200, 300},
		},
		Policy: []Calculation{
			{ID: "bonus_list", Expression: "vec_scale(bonus_rate, salaries)"}, // [200, 300, 400]
			{ID: "total_bonus", Expression: "vec_add(bonus_list, fixed_bonus)"}, // [300, 500, 700]
			{ID: "sum_total", Expression: "vec_sum(total_bonus)"},                  // 1500
		},
	}

	res, err := Compute(req)
	assert.NoError(t, err)
	assert.True(t, res.Decimals["sum_total"].Equal(decimal.NewFromInt(1500)))
}

func TestExa_TemporalOperations(t *testing.T) {
	req := Request{
		Policy: []Calculation{
			{ID: "yr", Expression: "year('2023-05-15')"},                  // 2023
			{ID: "mon", Expression: "month('2023-05-15')"},                // 5
			{ID: "dy", Expression: "day('2023-05-15')"},                   // 15
			{ID: "db", Expression: "days_between('2026-02-01', '2026-02-02')"}, // 2 (Inclusive)
			{ID: "dim1", Expression: "days_in_month('2023-02-01')"},       // 28
			{ID: "dim2", Expression: "days_in_month('2024-02-01')"},       // 29 (Leap year)
			// Manual Pro-rata example
			{ID: "pro_rata", Expression: "days_between('2023-02-01', '2023-02-14') / days_in_month('2023-02-01')"}, // 14/28 = 0.5
		},
	}
	res, err := Compute(req)
	assert.NoError(t, err)
	assert.True(t, res.Decimals["yr"].Equal(decimal.NewFromInt(2023)))
	assert.True(t, res.Decimals["mon"].Equal(decimal.NewFromInt(5)))
	assert.True(t, res.Decimals["dy"].Equal(decimal.NewFromInt(15)))
	assert.True(t, res.Decimals["db"].Equal(decimal.NewFromInt(2)))
	assert.True(t, res.Decimals["dim1"].Equal(decimal.NewFromInt(28)))
	assert.True(t, res.Decimals["dim2"].Equal(decimal.NewFromInt(29)))
	assert.True(t, res.Decimals["pro_rata"].Equal(decimal.RequireFromString("0.5")))
}

func TestExa_ProgramCaching(t *testing.T) {
	engine := NewEngine()
	req := Request{
		Inputs: map[string]any{"a": 10},
		Policy: []Calculation{{ID: "res", Expression: "a + 1"}},
	}
	
	// First run (compiles)
	_, err := engine.Compute(context.Background(), req)
	assert.NoError(t, err)
	
	// Second run (should hit cache)
	res, err := engine.Compute(context.Background(), req)
	assert.NoError(t, err)
	assert.True(t, res.Decimals["res"].Equal(decimal.NewFromInt(11)))
}
