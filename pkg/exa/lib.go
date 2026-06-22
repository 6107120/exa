package exa

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/functions"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/shopspring/decimal"
)

// Library implements cel.Library for Exa core functions.
type Library struct{}

func (Library) CompileOptions() []cel.EnvOption {
	opts := []cel.EnvOption{
		cel.Types(DecimalType),
	}

	mixedTypes := []*cel.Type{cel.IntType, cel.UintType, cel.DoubleType, cel.StringType, DecimalType}

	// 1. Arithmetic Operators
	for _, op := range []string{operators.Add, operators.Subtract, operators.Multiply, operators.Divide} {
		var overloads []cel.FunctionOpt
		for _, t1 := range mixedTypes {
			for _, t2 := range mixedTypes {
				if t1 == DecimalType || t2 == DecimalType {
					id := fmt.Sprintf("%s_%s_%s", op, t1.TypeName(), t2.TypeName())
					overloads = append(overloads, cel.Overload(id, []*cel.Type{t1, t2}, DecimalType))
				}
			}
		}
		opts = append(opts, cel.Function(op, overloads...))
	}

	// 2. Comparison Operators
	for _, op := range []string{operators.Greater, operators.Less, operators.GreaterEquals, operators.LessEquals} {
		var overloads []cel.FunctionOpt
		for _, t1 := range mixedTypes {
			for _, t2 := range mixedTypes {
				if t1 == DecimalType || t2 == DecimalType {
					id := fmt.Sprintf("%s_%s_%s", op, t1.TypeName(), t2.TypeName())
					overloads = append(overloads, cel.Overload(id, []*cel.Type{t1, t2}, cel.BoolType))
				}
			}
		}
		opts = append(opts, cel.Function(op, overloads...))
	}

	// 3. Builtin Functions
	unary := []string{"abs", "decimal"}
	for _, fn := range unary {
		opts = append(opts, cel.Function(fn, cel.Overload(fn+"_any", []*cel.Type{cel.DynType}, DecimalType)))
	}

	binary := []string{"min", "max", "pow", "round", "ceil", "round_down"}
	for _, fn := range binary {
		opts = append(opts, cel.Function(fn, cel.Overload(fn+"_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType)))
	}

	opts = append(opts,
		cel.Function("sum", cel.Overload("sum_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("assert", cel.Overload("assert_bool_string", []*cel.Type{cel.BoolType, cel.StringType}, cel.BoolType)),
	)

	return opts
}

func (Library) ProgramOptions() []cel.ProgramOption {
	var bindings []*functions.Overload

	// Suffixes for arithmetic/comparison
	mixedTypes := []*cel.Type{cel.IntType, cel.UintType, cel.DoubleType, cel.StringType, DecimalType}
	var suffixes []string
	for _, t1 := range mixedTypes {
		for _, t2 := range mixedTypes {
			if t1 == DecimalType || t2 == DecimalType {
				suffixes = append(suffixes, fmt.Sprintf("_%s_%s", t1.TypeName(), t2.TypeName()))
			}
		}
	}

	// Arithmetic Bindings
	arith := []struct {
		op string
		f  func(d1, d2 decimal.Decimal) decimal.Decimal
	}{
		{operators.Add, func(d1, d2 decimal.Decimal) decimal.Decimal { return d1.Add(d2) }},
		{operators.Subtract, func(d1, d2 decimal.Decimal) decimal.Decimal { return d1.Sub(d2) }},
		{operators.Multiply, func(d1, d2 decimal.Decimal) decimal.Decimal { return d1.Mul(d2) }},
	}
	for _, spec := range arith {
		for _, sfx := range suffixes {
			bindings = append(bindings, &functions.Overload{
				Operator: spec.op + sfx,
				Binary: func(lhs, rhs ref.Val) ref.Val {
					l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
					return NewDecimal(spec.f(l, r))
				},
			})
		}
	}

	// Division Binding
	for _, sfx := range suffixes {
		bindings = append(bindings, &functions.Overload{
			Operator: operators.Divide + sfx,
			Binary: func(lhs, rhs ref.Val) ref.Val {
				l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
				if r.IsZero() { return types.NewErr("division by zero") }
				return NewDecimal(l.Div(r))
			},
		})
	}

	// Comparison Bindings
	cmp := []struct {
		op string
		f  func(d1, d2 decimal.Decimal) bool
	}{
		{operators.Greater, func(d1, d2 decimal.Decimal) bool { return d1.GreaterThan(d2) }},
		{operators.Less, func(d1, d2 decimal.Decimal) bool { return d1.LessThan(d2) }},
		{operators.GreaterEquals, func(d1, d2 decimal.Decimal) bool { return d1.GreaterThanOrEqual(d2) }},
		{operators.LessEquals, func(d1, d2 decimal.Decimal) bool { return d1.LessThanOrEqual(d2) }},
	}
	for _, spec := range cmp {
		for _, sfx := range suffixes {
			bindings = append(bindings, &functions.Overload{
				Operator: spec.op + sfx,
				Binary: func(lhs, rhs ref.Val) ref.Val {
					l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
					return types.Bool(spec.f(l, r))
				},
			})
		}
	}

	// Builtin Function Bindings
	bindings = append(bindings,
		&functions.Overload{Operator: "abs_any", Unary: func(v ref.Val) ref.Val { d, _ := ToDecimal(v); return NewDecimal(d.Abs()) }},
		&functions.Overload{Operator: "decimal_any", Unary: func(v ref.Val) ref.Val { d, _ := ToDecimal(v); return NewDecimal(d) }},
		&functions.Overload{Operator: "min_any", Binary: func(l, r ref.Val) ref.Val {
			ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
			if ld.LessThan(rd) { return NewDecimal(ld) } else { return NewDecimal(rd) }
		}},
		&functions.Overload{Operator: "max_any", Binary: func(l, r ref.Val) ref.Val {
			ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
			if ld.GreaterThan(rd) { return NewDecimal(ld) } else { return NewDecimal(rd) }
		}},
		&functions.Overload{Operator: "pow_any", Binary: func(l, r ref.Val) ref.Val {
			ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
			return NewDecimal(ld.Pow(rd))
		}},
		&functions.Overload{Operator: "round_any", Binary: func(l, r ref.Val) ref.Val {
			ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
			return NewDecimal(ld.Round(int32(rd.IntPart())))
		}},
		&functions.Overload{Operator: "round_down_any", Binary: func(l, r ref.Val) ref.Val {
			ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
			return NewDecimal(ld.Truncate(int32(rd.IntPart())))
		}},
		&functions.Overload{Operator: "ceil_any", Binary: func(l, r ref.Val) ref.Val {
			ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
			p := int32(rd.IntPart()); shift := decimal.New(1, p)
			return NewDecimal(ld.Mul(shift).Ceil().Div(shift))
		}},
		&functions.Overload{Operator: "sum_any", Unary: sumBinding},
		&functions.Overload{Operator: "assert_bool_string", Binary: func(l, r ref.Val) ref.Val {
			if !bool(l.(types.Bool)) { return types.NewErr("assertion failed: %s", string(r.(types.String))) }
			return types.Bool(true)
		}},
	)

	return []cel.ProgramOption{cel.Functions(bindings...)}
}

func sumBinding(v ref.Val) ref.Val {
	list, ok := v.(traits.Lister)
	if !ok { return types.NewErr("sum() requires a list") }
	sum := decimal.Zero
	it := list.Iterator()
	for it.HasNext() == types.Bool(true) {
		d, err := ToDecimal(it.Next())
		if err != nil { return err }
		sum = sum.Add(d)
	}
	return NewDecimal(sum)
}
