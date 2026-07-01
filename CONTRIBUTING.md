# Contributing to Exa

Thank you for your interest in contributing to Exa! Exa is a high-precision, CEL-based calculation engine designed to execute dynamic formulas with strict decimal correctness while offering a seamless, excel-like user experience.

This guide outlines our project philosophy, contribution workflows, and strict development guidelines for adding new functions or operators.

---

## 1. Project Philosophy & Core Architecture

Exa's architecture relies on the following two key pillars:
1. **No Explicit Casting**: Users should not need to wrap formulas in casting functions like `decimal(...)` unless absolutely necessary. The engine automatically handles type mismatches behind the scenes.
2. **Implicit Coercion**: Operations involving a mix of custom high-precision `Decimal` values and standard Go/CEL primitives (such as `int`, `double`, or `dyn`) are dynamically promoted to `Decimal` at runtime without losing precision.

---

## 2. Guidelines for Adding Functions and Operators

To prevent runtime errors (such as `no such overload` or type mismatches) and ensure structural elegance, all new functions and operators must adhere to these three standard protocols.

### Protocol 1. Implicit Casting & Type Unification
* **Parameter Laxity**: Declare function arguments as `cel.DynType` at compile time so that they can receive integers, floats, strings, or decimal values.
* **Runtime Elevation**: Inside the runtime binding implementation, run all numeric operands through the `ToDecimal(val)` helper to safely coerce them into `decimal.Decimal`.
* **Result Unification**: All math/calculation results must be returned as a `*Decimal` instance using the `NewDecimal(...)` wrapper.

#### Code Pattern (Standard Function):
```go
cel.Function("my_math_func",
    cel.Overload("my_math_func_any", []*cel.Type{cel.DynType}, DecimalType,
        cel.UnaryBinding(func(v ref.Val) ref.Val {
            d, err := ToDecimal(v) // Safely elevates int, float, string, or Decimal
            if err != nil { return err }
            return NewDecimal(d.Abs()) // Always return custom DecimalType
        }),
    ),
)
```

---

### Protocol 2. Operator Declaration-Implementation Split & Hijacking
CEL resolves standard operators (like `+`, `-`, `*`, `/`, `<`, `<=`, `>`, `>=`) using static overload IDs at compile time. To avoid dispatch issues when operating on dynamic variables (like map lookups):

1. **CompileOptions (Signature Only)**:
   * Define only the signature declarations (using `cel.Overload`) in `CompileOptions()`.
   * **Do not** bind runtime implementations here (e.g., avoid `cel.BinaryBinding`).
   * Declare custom asymmetric signatures (like `[Decimal, dyn]` and `[int, Decimal]`) to satisfy the type checker.

2. **ProgramOptions (Binding & Hijacking)**:
   * Provide the execution bindings inside `ProgramOptions()` via `cel.Functions`.
   * **Crucial**: You must hijack the standard CEL builtin overload IDs (`{prefix}_int64`, `{prefix}_uint64`, `{prefix}_double`) to ensure they evaluate as Decimals if either operand is promoted.

#### Code Pattern (Operator Expansion):
```go
// 1. In CompileOptions(): Declare signatures
cel.Function(operators.Add,
    cel.Overload("add_decimal_dyn", []*cel.Type{DecimalType, cel.DynType}, DecimalType),
    cel.Overload("add_int_decimal", []*cel.Type{cel.IntType, DecimalType}, DecimalType),
)

// 2. In ProgramOptions(): Register bindings and hijack builtins
overloads = append(overloads, &functions.Overload{
    Operator: "add_decimal_dyn",
    Binary: func(lhs, rhs ref.Val) ref.Val {
        l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
        return NewDecimal(l.Add(r))
    },
})
for _, bName := range []string{"int64", "uint64", "double"} {
    overloads = append(overloads, &functions.Overload{
        Operator: fmt.Sprintf("add_%s", bName), // Hijack CEL standard "add_int64", etc.
        Binary: func(lhs, rhs ref.Val) ref.Val {
            l, _ := ToDecimal(lhs); r, _ := ToDecimal(rhs)
            return NewDecimal(l.Add(r))
        },
    })
}
```

---

### Protocol 3. Integer Boundary Isolation
* **Pure Integer Exceptions**: Functions that naturally return structural metadata or integer identifiers—such as `size(list)` (list length) or date components like `year(date)`—should maintain their native standard `int` return type.
* **Why?**: This prevents breaking operations that strictly expect standard integers (e.g., list index access: `list[size(list) - 1]`).
* **Coercion Delegation**: Let the hijacked operator bindings (Protocol 2) handle the promotion automatically if these integers are subsequently used in math calculations (e.g. `size(segments) / days`).

---

## 3. Development & Testing Workflow

Before submitting a Pull Request, ensure that all tests pass and your code does not introduce regressions.

### Running Tests
Execute the tests locally within the workspace directory:
```bash
go test -v ./pkg/exa
```

### Adding Tests
When introducing a new function or operator, add appropriate test cases in:
* `pkg/exa/engine_test.go` for standard flows and builtins.
* `pkg/exa/robustness_test.go` for mixed-type boundaries and precision check edge cases.

---

## 4. Coding Standards

* Keep responses and documentation concise.
* Preserve existing comments and docstrings.
* Use idiomatic Go formatting (`go fmt`).
* Make sure all math calculations continue to run with high-precision shopspring/decimal backing.
