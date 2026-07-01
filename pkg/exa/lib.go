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

	lhsTypes := []*cel.Type{
		cel.IntType,
		cel.UintType,
		cel.DoubleType,
		cel.StringType,
		cel.BytesType,
		cel.DurationType,
		cel.TimestampType,
		cel.BoolType,
		cel.ListType(cel.DynType),
		cel.MapType(cel.DynType, cel.DynType),
	}
	typeNames := []string{"int", "uint", "double", "string", "bytes", "duration", "timestamp", "bool", "list", "map"}

	// 1. Arithmetic Operators (Declarations only in CompileOptions)
	arithOps := []struct {
		op string
	}{
		{operators.Add},
		{operators.Subtract},
		{operators.Multiply},
	}

	for _, spec := range arithOps {
		op := spec.op
		
		overloads := []cel.FunctionOpt{
			cel.Overload(fmt.Sprintf("%s_decimal_dyn", op), []*cel.Type{DecimalType, cel.DynType}, DecimalType),
		}
		
		for idx, t := range lhsTypes {
			tName := typeNames[idx]
			overloads = append(overloads,
				cel.Overload(fmt.Sprintf("%s_%s_decimal", op, tName), []*cel.Type{t, DecimalType}, DecimalType),
			)
		}
		
		opts = append(opts, cel.Function(op, overloads...))
	}

	// Division Operator with asymmetric overloads
	divOverloads := []cel.FunctionOpt{
		cel.Overload("divide_decimal_dyn", []*cel.Type{DecimalType, cel.DynType}, DecimalType),
	}
	for idx, t := range lhsTypes {
		tName := typeNames[idx]
		divOverloads = append(divOverloads,
			cel.Overload(fmt.Sprintf("divide_%s_decimal", tName), []*cel.Type{t, DecimalType}, DecimalType),
		)
	}
	opts = append(opts, cel.Function(operators.Divide, divOverloads...))

	// 2. Comparison Operators (Declarations only in CompileOptions)
	cmpOps := []struct {
		op string
	}{
		{operators.Greater},
		{operators.Less},
		{operators.GreaterEquals},
		{operators.LessEquals},
	}

	for _, spec := range cmpOps {
		op := spec.op
		
		overloads := []cel.FunctionOpt{
			cel.Overload(fmt.Sprintf("%s_decimal_dyn", op), []*cel.Type{DecimalType, cel.DynType}, cel.BoolType),
		}
		
		for idx, t := range lhsTypes {
			tName := typeNames[idx]
			overloads = append(overloads,
				cel.Overload(fmt.Sprintf("%s_%s_decimal", op, tName), []*cel.Type{t, DecimalType}, cel.BoolType),
			)
		}
		
		opts = append(opts, cel.Function(op, overloads...))
	}

	// 3. Builtin Functions (Inline bindings for custom non-operator functions)
	opts = append(opts,
		cel.Function("abs",
			cel.Overload("abs_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					d, _ := ToDecimal(v)
					return NewDecimal(d.Abs())
				}),
			),
		),
		cel.Function("decimal",
			cel.Overload("decimal_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					d, err := ToDecimal(v)
					if err != nil { return err }
					return NewDecimal(d)
				}),
			),
		),
		cel.Function("min",
			cel.Overload("min_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType,
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
					if ld.LessThan(rd) { return NewDecimal(ld) }
					return NewDecimal(rd)
				}),
			),
		),
		cel.Function("max",
			cel.Overload("max_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType,
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
					if ld.GreaterThan(rd) { return NewDecimal(ld) }
					return NewDecimal(rd)
				}),
			),
		),
		cel.Function("pow",
			cel.Overload("pow_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType,
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
					return NewDecimal(ld.Pow(rd))
				}),
			),
		),
		cel.Function("round",
			cel.Overload("round_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType,
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
					return NewDecimal(ld.Round(int32(rd.IntPart())))
				}),
			),
		),
		cel.Function("round_down",
			cel.Overload("round_down_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType,
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
					return NewDecimal(ld.Truncate(int32(rd.IntPart())))
				}),
			),
		),
		cel.Function("ceil",
			cel.Overload("ceil_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType,
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					ld, _ := ToDecimal(l); rd, _ := ToDecimal(r)
					p := int32(rd.IntPart()); shift := decimal.New(1, p)
					return NewDecimal(ld.Mul(shift).Ceil().Div(shift))
				}),
			),
		),
		cel.Function("sum",
			cel.Overload("sum_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(sumBinding),
			),
		),
		cel.Function("vec_sum",
			cel.Overload("vec_sum_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(sumBinding),
			),
		),
		cel.Function("assert",
			cel.Overload("assert_bool_string", []*cel.Type{cel.BoolType, cel.StringType}, cel.BoolType,
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					if !bool(l.(types.Bool)) { return types.NewErr("assertion failed: %s", string(r.(types.String))) }
					return types.Bool(true)
				}),
			),
		),
		cel.Function("vec_add",
			cel.Overload("vec_add_list_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType),
				cel.BinaryBinding(func(l, r ref.Val) ref.Val { return vecOp(l, r, func(a, b decimal.Decimal) decimal.Decimal { return a.Add(b) }) }),
			),
		),
		cel.Function("vec_sub",
			cel.Overload("vec_sub_list_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType),
				cel.BinaryBinding(func(l, r ref.Val) ref.Val { return vecOp(l, r, func(a, b decimal.Decimal) decimal.Decimal { return a.Sub(b) }) }),
			),
		),
		cel.Function("vec_mul",
			cel.Overload("vec_mul_list_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType),
				cel.BinaryBinding(func(l, r ref.Val) ref.Val { return vecOp(l, r, func(a, b decimal.Decimal) decimal.Decimal { return a.Mul(b) }) }),
			),
		),
		cel.Function("vec_div",
			cel.Overload("vec_div_list_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType),
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					return vecOp(l, r, func(a, b decimal.Decimal) decimal.Decimal {
						if b.IsZero() { return decimal.Zero }
						return a.Div(b)
					})
				}),
			),
		),
		cel.Function("vec_scale",
			cel.Overload("vec_scale_dec_list", []*cel.Type{cel.DynType, cel.DynType}, cel.ListType(DecimalType),
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
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
				}),
			),
		),
		cel.Function("year",
			cel.Overload("year_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }
					return NewDecimal(decimal.NewFromInt(int64(t.Year())))
				}),
			),
		),
		cel.Function("month",
			cel.Overload("month_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }
					return NewDecimal(decimal.NewFromInt(int64(t.Month())))
				}),
			),
		),
		cel.Function("day",
			cel.Overload("day_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }
					return NewDecimal(decimal.NewFromInt(int64(t.Day())))
				}),
			),
		),
		cel.Function("hour",
			cel.Overload("hour_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }
					return NewDecimal(decimal.NewFromInt(int64(t.Hour())))
				}),
			),
		),
		cel.Function("minute",
			cel.Overload("minute_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }
					return NewDecimal(decimal.NewFromInt(int64(t.Minute())))
				}),
			),
		),
		cel.Function("second",
			cel.Overload("second_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					t, err := toTime(v); if err != nil { return types.NewErr("%v", err) }
					return NewDecimal(decimal.NewFromInt(int64(t.Second())))
				}),
			),
		),
		cel.Function("days_in_month",
			cel.Overload("days_in_month_any", []*cel.Type{cel.DynType}, DecimalType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					t, err := toTime(v)
					if err != nil { return types.NewErr("%v", err) }
					days := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
					return NewDecimal(decimal.NewFromInt(int64(days)))
				}),
			),
		),
		cel.Function("days_between",
			cel.Overload("days_between_any_any", []*cel.Type{cel.DynType, cel.DynType}, DecimalType,
				cel.BinaryBinding(func(l, r ref.Val) ref.Val {
					t1, err1 := toTime(l); t2, err2 := toTime(r)
					if err1 != nil { return types.NewErr("days_between arg1: %v", err1) }
					if err2 != nil { return types.NewErr("days_between arg2: %v", err2) }
					diff := (t2.Sub(t1).Hours() / 24) + 1
					return NewDecimal(decimal.NewFromFloat(diff))
				}),
			),
		),
	)

	return opts
}

func (Library) ProgramOptions() []cel.ProgramOption {
	var overloads []*functions.Overload

	typeNames := []string{"int", "uint", "double", "string", "bytes", "duration", "timestamp", "bool", "list", "map"}
	builtins := []string{"int64", "uint64", "double"}

	// 1. Arithmetic Operators bindings (including builtins hijacking)
	arithOps := []struct {
		op     string
		prefix string
		f      func(d1, d2 decimal.Decimal) decimal.Decimal
	}{
		{operators.Add, "add", func(d1, d2 decimal.Decimal) decimal.Decimal { return d1.Add(d2) }},
		{operators.Subtract, "subtract", func(d1, d2 decimal.Decimal) decimal.Decimal { return d1.Sub(d2) }},
		{operators.Multiply, "multiply", func(d1, d2 decimal.Decimal) decimal.Decimal { return d1.Mul(d2) }},
	}

	for _, spec := range arithOps {
		op := spec.op
		prefix := spec.prefix
		f := spec.f

		overloads = append(overloads, &functions.Overload{
			Operator: fmt.Sprintf("%s_decimal_dyn", op),
			Binary: func(lhs, rhs ref.Val) ref.Val {
				l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
				return NewDecimal(f(l, r))
			},
		})

		for _, tName := range typeNames {
			overloads = append(overloads, &functions.Overload{
				Operator: fmt.Sprintf("%s_%s_decimal", op, tName),
				Binary: func(lhs, rhs ref.Val) ref.Val {
					l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
					return NewDecimal(f(l, r))
				},
			})
		}

		// Hijack standard operators to ensure that dyn + decimal or int + dyn evaluates cleanly as Decimal
		for _, bName := range builtins {
			overloads = append(overloads, &functions.Overload{
				Operator: fmt.Sprintf("%s_%s", prefix, bName),
				Binary: func(lhs, rhs ref.Val) ref.Val {
					l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
					return NewDecimal(f(l, r))
				},
			})
		}
	}

	// Division Operator with zero division guard (including builtins hijacking)
	overloads = append(overloads, &functions.Overload{
		Operator: "divide_decimal_dyn",
		Binary: func(lhs, rhs ref.Val) ref.Val {
			l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
			if r.IsZero() { return types.NewErr("division by zero") }
			return NewDecimal(l.Div(r))
		},
	})

	for _, tName := range typeNames {
		overloads = append(overloads, &functions.Overload{
			Operator: fmt.Sprintf("divide_%s_decimal", tName),
			Binary: func(lhs, rhs ref.Val) ref.Val {
				l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
				if r.IsZero() { return types.NewErr("division by zero") }
				return NewDecimal(l.Div(r))
			},
		})
	}

	for _, bName := range builtins {
		overloads = append(overloads, &functions.Overload{
			Operator: fmt.Sprintf("divide_%s", bName),
			Binary: func(lhs, rhs ref.Val) ref.Val {
				l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
				if r.IsZero() { return types.NewErr("division by zero") }
				return NewDecimal(l.Div(r))
			},
		})
	}

	// 2. Comparison Operators bindings (including builtins hijacking)
	cmpOps := []struct {
		op     string
		prefix string
		f      func(d1, d2 decimal.Decimal) bool
	}{
		{operators.Greater, "greater", func(d1, d2 decimal.Decimal) bool { return d1.GreaterThan(d2) }},
		{operators.Less, "less", func(d1, d2 decimal.Decimal) bool { return d1.LessThan(d2) }},
		{operators.GreaterEquals, "greater_equals", func(d1, d2 decimal.Decimal) bool { return d1.GreaterThanOrEqual(d2) }},
		{operators.LessEquals, "less_equals", func(d1, d2 decimal.Decimal) bool { return d1.LessThanOrEqual(d2) }},
	}

	for _, spec := range cmpOps {
		op := spec.op
		prefix := spec.prefix
		f := spec.f

		overloads = append(overloads, &functions.Overload{
			Operator: fmt.Sprintf("%s_decimal_dyn", op),
			Binary: func(lhs, rhs ref.Val) ref.Val {
				l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
				return types.Bool(f(l, r))
			},
		})

		for _, tName := range typeNames {
			overloads = append(overloads, &functions.Overload{
				Operator: fmt.Sprintf("%s_%s_decimal", op, tName),
				Binary: func(lhs, rhs ref.Val) ref.Val {
					l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
					return types.Bool(f(l, r))
				},
			})
		}

		for _, bName := range builtins {
			overloads = append(overloads, &functions.Overload{
				Operator: fmt.Sprintf("%s_%s", prefix, bName),
				Binary: func(lhs, rhs ref.Val) ref.Val {
					l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
					return types.Bool(f(l, r))
				},
			})
		}
	}

	return []cel.ProgramOption{
		cel.Functions(overloads...),
	}
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
