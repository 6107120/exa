package exa

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// The original handoff bug: concrete-Int LHS (from size()) op a top-level numeric
// input. Previously "no such overload"; now resolves via DecimalType-typed inputs.
func TestFix_BareScalarAllOperators(t *testing.T) {
	in := map[string]any{"days": 28, "segs": []any{1, 2, 3}}
	cases := map[string]string{
		"add": "size(segs) + days", "sub": "size(segs) - days",
		"mul": "size(segs) * days", "div": "size(segs) / days",
		"gt": "size(segs) > days", "lt": "size(segs) < days",
		"gte": "size(segs) >= days", "lte": "size(segs) <= days",
	}
	for name, expr := range cases {
		t.Run(name, func(t *testing.T) {
			rs, err := NewEngine().Compute(context.Background(), Request{
				Inputs: in,
				Policy: []Calculation{{ID: "x", Expression: expr}},
			})
			assert.NoError(t, err, expr)
			_, dec := rs.Decimals["x"]
			_, b := rs.Bools["x"]
			assert.True(t, dec || b, "expected a decimal or bool result for %q", expr)
		})
	}
}

func TestFix_BareScalarValues(t *testing.T) {
	rs, err := NewEngine().Compute(context.Background(), Request{
		Inputs: map[string]any{"days": 28, "segs": []any{1, 2, 3}},
		Policy: []Calculation{
			{ID: "ratio", Expression: "size(segs) / days"},
			{ID: "over", Expression: "size(segs) > days"},
		},
	})
	assert.NoError(t, err)
	assert.True(t, rs.Decimals["ratio"].Equal(decimal.NewFromInt(3).Div(decimal.NewFromInt(28))))
	assert.Equal(t, false, rs.Bools["over"])
}

// (B) String values like "M" now surface instead of being dropped.
func TestFix_StringPassthrough(t *testing.T) {
	rs, err := NewEngine().Compute(context.Background(), Request{
		Inputs: map[string]any{
			"cash_comp_settlement_unit":  "D",
			"leave_comp_settlement_unit": "M",
			"statutory_work_method":      "MFWH",
			"work_type_id":               "standard_flexible",
			"daily_contractual_minutes":  480,
		},
		Policy: []Calculation{
			{ID: "cash_unit", Expression: "cash_comp_settlement_unit"},
			{ID: "leave_unit", Expression: "leave_comp_settlement_unit"},
			{ID: "method", Expression: "statutory_work_method"},
			{ID: "wtype", Expression: "work_type_id"},
			{ID: "daily", Expression: "daily_contractual_minutes"}, // numeric passthrough
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "D", rs.Strings["cash_unit"])
	assert.Equal(t, "M", rs.Strings["leave_unit"])
	assert.Equal(t, "MFWH", rs.Strings["method"])
	assert.Equal(t, "standard_flexible", rs.Strings["wtype"])
	// numeric passthrough still lands in Decimals, not Strings
	assert.True(t, rs.Decimals["daily"].Equal(decimal.NewFromInt(480)))
	_, inStrings := rs.Strings["daily"]
	assert.False(t, inStrings)
}

// Regression: pure int/int division must remain high-precision decimal division,
// not truncating integer division.
func TestFix_IntIntDivisionStaysDecimal(t *testing.T) {
	rs, err := NewEngine().Compute(context.Background(), Request{
		Inputs: map[string]any{"a": []any{1, 2, 3}, "b": []any{1, 2}},
		Policy: []Calculation{{ID: "x", Expression: "size(a) / size(b)"}}, // 3/2
	})
	assert.NoError(t, err)
	assert.True(t, rs.Decimals["x"].Equal(decimal.RequireFromString("1.5")), "got %v", rs.Decimals["x"])
}

// Compute returns the unified Result with a populated Decimals map.
func TestFix_ComputeResultShape(t *testing.T) {
	res, err := NewEngine().Compute(context.Background(), Request{
		Inputs: map[string]any{"days": 28, "segs": []any{1, 2, 3}},
		Policy: []Calculation{{ID: "x", Expression: "size(segs) / days"}},
	})
	assert.NoError(t, err)
	assert.Contains(t, res.Decimals, "x")
}
