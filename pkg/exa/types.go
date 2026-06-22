package exa

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/shopspring/decimal"
)

// DecimalType is the custom Decimal data type registered in the CEL environment.
var DecimalType = cel.ObjectType("exa.Decimal",
	traits.ComparerType,
	traits.AdderType,
	traits.SubtractorType,
	traits.MultiplierType,
	traits.DividerType,
)

// Registry implements types.Adapter and types.Registry.
type Registry struct {
	ref.TypeRegistry
}

func NewRegistry() *Registry {
	reg, _ := types.NewRegistry()
	return &Registry{TypeRegistry: reg}
}

func (r *Registry) NativeToValue(v any) ref.Val {
	switch val := v.(type) {
	case decimal.Decimal:
		return NewDecimal(val)
	case *decimal.Decimal:
		if val == nil { return types.NullValue }
		return NewDecimal(*val)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		d, _ := decimal.NewFromString(fmt.Sprint(val))
		return NewDecimal(d)
	case float32, float64:
		return NewDecimal(decimal.NewFromFloat(reflect.ValueOf(val).Float()))
	case json.Number:
		d, err := decimal.NewFromString(string(val))
		if err == nil { return NewDecimal(d) }
		return types.DefaultTypeAdapter.NativeToValue(val)
	case string:
		if d, err := decimal.NewFromString(val); err == nil {
			return NewDecimal(d)
		}
		return types.String(val)
	case []any:
		res := make([]any, len(val))
		for i, x := range val {
			res[i] = r.NativeToValue(x)
		}
		return r.TypeRegistry.NativeToValue(res)
	case map[string]any:
		res := make(map[string]any)
		for k, x := range val {
			res[k] = r.NativeToValue(x)
		}
		return r.TypeRegistry.NativeToValue(res)
	default:
		return r.TypeRegistry.NativeToValue(v)
	}
}

// Decimal wraps shopspring/decimal.Decimal to implement CEL's ref.Val interface.
type Decimal struct {
	decimal.Decimal
}

// NewDecimal creates a new Decimal wrapper.
func NewDecimal(d decimal.Decimal) *Decimal {
	return &Decimal{Decimal: d}
}

// ConvertToNative supports conversion to shopspring/decimal.Decimal.
func (d *Decimal) ConvertToNative(typeDesc reflect.Type) (any, error) {
	if typeDesc == reflect.TypeOf(decimal.Decimal{}) {
		return d.Decimal, nil
	}
	if typeDesc == reflect.TypeOf(Decimal{}) || typeDesc == reflect.TypeOf(&Decimal{}) {
		return d, nil
	}
	if typeDesc == reflect.TypeOf(float64(0)) {
		f, _ := d.Decimal.Float64()
		return f, nil
	}
	return nil, fmt.Errorf("cannot convert Decimal to %v", typeDesc)
}

// ConvertToType implements the CEL ref.Val interface for type casting.
func (d *Decimal) ConvertToType(typeVal ref.Type) ref.Val {
	if typeVal == DecimalType {
		return d
	}
	if typeVal == types.StringType {
		return types.String(d.String())
	}
	return types.NewErr("type conversion error from Decimal to %s", typeVal.TypeName())
}

// ToDecimal is a helper to convert various CEL types to decimal.Decimal.
func ToDecimal(val ref.Val) (decimal.Decimal, ref.Val) {
	switch v := val.(type) {
	case *Decimal:
		return v.Decimal, nil
	case types.Int:
		return decimal.NewFromInt(int64(v)), nil
	case types.Uint:
		return decimal.NewFromUint64(uint64(v)), nil
	case types.String:
		d, err := decimal.NewFromString(string(v))
		if err != nil {
			return decimal.Zero, types.NewErr("invalid decimal format: %s", err.Error())
		}
		return d, nil
	case types.Double:
		return decimal.NewFromFloat(float64(v)), nil
	case types.Bytes:
		d, err := decimal.NewFromString(string(v))
		if err != nil {
			return decimal.Zero, types.NewErr("bytes conversion failed: %s", err.Error())
		}
		return d, nil
	default:
		return decimal.Zero, types.NewErr("unsupported type for decimal: %s", val.Type().TypeName())
	}
}

func (d *Decimal) Equal(other ref.Val) ref.Val {
	otherDec, err := ToDecimal(other)
	if err != nil {
		return types.Bool(false)
	}
	return types.Bool(d.Decimal.Equal(otherDec))
}

func (d *Decimal) Compare(other ref.Val) ref.Val {
	otherDec, err := ToDecimal(other)
	if err != nil {
		return err
	}
	return types.Int(d.Decimal.Cmp(otherDec))
}

func (d *Decimal) Add(other ref.Val) ref.Val {
	otherDec, err := ToDecimal(other)
	if err != nil {
		return err
	}
	return &Decimal{Decimal: d.Decimal.Add(otherDec)}
}

func (d *Decimal) Subtract(other ref.Val) ref.Val {
	otherDec, err := ToDecimal(other)
	if err != nil {
		return err
	}
	return &Decimal{Decimal: d.Decimal.Sub(otherDec)}
}

func (d *Decimal) Multiply(other ref.Val) ref.Val {
	otherDec, err := ToDecimal(other)
	if err != nil {
		return err
	}
	return &Decimal{Decimal: d.Decimal.Mul(otherDec)}
}

func (d *Decimal) Divide(other ref.Val) ref.Val {
	otherDec, err := ToDecimal(other)
	if err != nil {
		return err
	}
	if otherDec.IsZero() {
		return types.NewErr("division by zero")
	}
	return &Decimal{Decimal: d.Decimal.Div(otherDec)}
}

func (d *Decimal) Type() ref.Type {
	return DecimalType
}

func (d *Decimal) Value() any {
	return d.Decimal
}
