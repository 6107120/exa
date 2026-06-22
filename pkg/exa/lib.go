package exa

import (
	"fmt"

	"time"
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
		cel.Function("vec_sum", cel.Overload("vec_sum_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("assert", cel.Overload("assert_bool_string", []*cel.Type{cel.BoolType, cel.StringType}, cel.BoolType)),
		// Vector (List) Operations
		cel.Function("vec_add", cel.Overload("vec_add_list_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType))),
		cel.Function("vec_sub", cel.Overload("vec_sub_list_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType))),
		cel.Function("vec_mul", cel.Overload("vec_mul_list_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType))),
		cel.Function("vec_div", cel.Overload("vec_div_list_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType))),
		cel.Function("vec_scale", cel.Overload("vec_scale_dec_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType))),
		// Temporal Primitives
		cel.Function("year", cel.Overload("year_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("month", cel.Overload("month_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("day", cel.Overload("day_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("hour", cel.Overload("hour_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("minute", cel.Overload("minute_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("second", cel.Overload("second_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("days_in_month", cel.Overload("days_in_month_any", []*cel.Type{cel.DynType}, DecimalType)),
		cel.Function("days_between", cel.Overload("days_between_any_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType)),
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
		&functions.Overload{Operator: "vec_sum_any", Unary: sumBinding},
		&functions.Overload{Operator: "assert_bool_string", Binary: func(l, r ref.Val) ref.Val {
			if !bool(l.(types.Bool)) { return types.NewErr("assertion failed: %s", string(r.(types.String))) }
			return types.Bool(true)
		}},
		// Vector Bindings
		&functions.Overload{Operator: "vec_add_list_list", Binary: func(l, r ref.Val) ref.Val { return vecOp(l, r, func(a, b decimal.Decimal) decimal.Decimal { return a.Add(b) }) }},
		&functions.Overload{Operator: "vec_sub_list_list", Binary: func(l, r ref.Val) ref.Val { return vecOp(l, r, func(a, b decimal.Decimal) decimal.Decimal { return a.Sub(b) }) }},
		&functions.Overload{Operator: "vec_mul_list_list", Binary: func(l, r ref.Val) ref.Val { return vecOp(l, r, func(a, b decimal.Decimal) decimal.Decimal { return a.Mul(b) }) }},
		&functions.Overload{Operator: "vec_div_list_list", Binary: func(l, r ref.Val) ref.Val { 
			return vecOp(l, r, func(a, b decimal.Decimal) decimal.Decimal {
				if b.IsZero() { return decimal.Zero } // Or return error? For vectors, let's keep it safe or use a NaN-like approach. 
				return a.Div(b)
			})
		}},
		&functions.Overload{Operator: "vec_scale_dec_list", Binary: func(l, r ref.Val) ref.Val {
			base, err := ToDecimal(l)
			if err != nil { return types.NewErr("vec_scale first arg must be decimal: %v", err) }
			mask, ok := r.(traits.Lister)
			if !ok { return types.NewErr("vec_scale second arg must be a list") }
			
			size := int(mask.Size().(types.Int))
			res := make([]ref.Val, size)
			it := mask.Iterator()
			for i := 0; it.HasNext() == types.Bool(true); i++ {
				w, err := ToDecimal(it.Next())
				if err != nil { return err }
				res[i] = NewDecimal(base.Mul(w))
			}
			return NewRegistry().NativeToValue(res)
		}},
		// Temporal Bindings
		&functions.Overload{Operator: "year_any", Unary: func(v ref.Val) ref.Val { t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }; return NewDecimal(decimal.NewFromInt(int64(t.Year()))) }},
		&functions.Overload{Operator: "month_any", Unary: func(v ref.Val) ref.Val { t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }; return NewDecimal(decimal.NewFromInt(int64(t.Month()))) }},
		&functions.Overload{Operator: "day_any", Unary: func(v ref.Val) ref.Val { t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }; return NewDecimal(decimal.NewFromInt(int64(t.Day()))) }},
		&functions.Overload{Operator: "hour_any", Unary: func(v ref.Val) ref.Val { t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }; return NewDecimal(decimal.NewFromInt(int64(t.Hour()))) }},
		&functions.Overload{Operator: "minute_any", Unary: func(v ref.Val) ref.Val { t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }; return NewDecimal(decimal.NewFromInt(int64(t.Minute()))) }},
		&functions.Overload{Operator: "second_any", Unary: func(v ref.Val) ref.Val { t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }; return NewDecimal(decimal.NewFromInt(int64(t.Second()))) }},
		&functions.Overload{Operator: "days_in_month_any", Unary: func(v ref.Val) ref.Val {
			t, err := toTime(v)
			if err != nil { return types.NewErr("%v", err) }
			// Last day of month is the 0th day of the next month
			days := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
			return NewDecimal(decimal.NewFromInt(int64(days)))
		}},
		&functions.Overload{Operator: "days_between_any_any", Binary: func(l, r ref.Val) ref.Val {
			t1, err1 := toTime(l); t2, err2 := toTime(r)
			if err1 != nil { return types.NewErr("days_between arg1: %v", err1) }
			if err2 != nil { return types.NewErr("days_between arg2: %v", err2) }
			// Inclusive: Add 1 day to the difference
			diff := (t2.Sub(t1).Hours() / 24) + 1
			return NewDecimal(decimal.NewFromFloat(diff))
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

func vecOp(l, r ref.Val, f func(a, b decimal.Decimal) decimal.Decimal) ref.Val {
	lList, ok1 := l.(traits.Lister)
	rList, ok2 := r.(traits.Lister)
	if !ok1 || !ok2 { return types.NewErr("vector operations require two lists") }
	if lList.Size() != rList.Size() { return types.NewErr("vector size mismatch") }
	
	size := int(lList.Size().(types.Int))
	res := make([]ref.Val, size)
	lIt := lList.Iterator()
	rIt := rList.Iterator()
	for i := 0; lIt.HasNext() == types.Bool(true); i++ {
		a, errA := ToDecimal(lIt.Next())
		b, errB := ToDecimal(rIt.Next())
		if errA != nil { return errA }
		if errB != nil { return errB }
		res[i] = NewDecimal(f(a, b))
	}
	return NewRegistry().NativeToValue(res)
}

func toTime(v ref.Val) (time.Time, error) {
	if t, ok := v.Value().(time.Time); ok { return t, nil }
	if s, ok := v.Value().(string); ok {
		formats := []string{"2006-01-02", "2006-01-02T15:04:05Z", time.RFC3339}
		for _, f := range formats {
			if t, err := time.Parse(f, s); err == nil { return t, nil }
		}
	}
	return time.Time{}, fmt.Errorf("unsupported date format: %v", v.Value())
}
