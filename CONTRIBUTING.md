# Contributing to Exa

Exa is a high-precision, CEL-based calculation engine that executes dynamic formulas with strict decimal correctness and an "excel-like" authoring experience — plain values in, plain formulas in, precise results out.

This guide gives you the mental model to contribute effectively: the **execution pipeline**, the **type & coercion model**, and — most importantly — the **operator overload-resolution model** that governs why arithmetic on mixed types either works or fails. Read §3 before touching any operator.

---

## 1. Guarantees We Preserve

1. **No explicit casting.** Users never wrap operands in `decimal(...)` or declare types. `size(segs) / days` must "just work".
2. **Precision everywhere.** Every numeric value is backed by `shopspring/decimal`. No IEEE-754 drift, ever.
3. **Self-contained arithmetic.** Correctness must not depend on which operand happens to be on the left, or whether an expression is written flat vs. nested.

A change that regresses any of these is a bug, no matter how clean it looks.

---

## 2. Execution Pipeline

A single `Compute` call flows through these stages (`pkg/exa/kernel.go` unless noted):

| # | Stage | What happens |
|---|-------|--------------|
| 1 | **Normalize** (`transpile.go`) | Unicode NFC + strip invisible/control chars from idents & string literals. |
| 2 | **Transpile** (`transpile.go`) | If non-ASCII identifiers are present, encode them to ASCII-safe names (decoded back at the end). Pure-ASCII requests skip this. |
| 3 | **Env build** (`getEnv`) | Declare each **input** variable with a concrete type (numeric → `DecimalType`, else `DynType`) and each **policy id** as `DynType`. Cached by an env signature that includes input type codes. |
| 4 | **Literal optimization** (`literalOptimizer`) | Rewrite every numeric literal `N` into `decimal("N")` so literals are `Decimal` from the start. |
| 5 | **DAG sort** (`sortByDependencies`) | Derive execution order by scanning identifiers referenced in each checked AST. Cycles → error. |
| 6 | **Evaluate** | Run nodes in order; compiled programs are cached by `envSignature + expression`. |
| 7 | **Collect** | Partition each result by runtime type into `Result{Decimals, Strings, Bools}` (§4). |

---

## 3. The Operator Overload-Resolution Model  ⭐

This is the non-obvious core. Custom types (`*Decimal`) interoperate with builtin CEL types (`int` from `size()`, `dyn` from map/list lookups) **only** because of how CEL resolves and dispatches operator calls.

### 3.1 How CEL picks an implementation

For a call like `a / b`, the **type checker** matches the operand types against every declared overload:

- **Exactly one** candidate survives → the planner records that overload id and calls **its registered binding** directly.
- **Several** candidates survive (typically because an operand is `dyn`, which is assignable to everything) → the planner records **no** id and falls back to the operator's **standard-library singleton**, which dispatches on the **left operand's trait** (`lhs.(traits.Divider).Divide(rhs)`).

That fallback is the trap. If the LHS is a builtin `types.Int` (e.g. `size(x)`) and the RHS is a `*Decimal`, the singleton calls `Int.Divide(Decimal)` — and `Int` has no idea what a `Decimal` is → **`no such overload`**.

### 3.2 Why our `*Decimal` is on both sides

`*Decimal` (`types.go`) implements `Adder / Subtractor / Multiplier / Divider / Comparer`, and each method funnels the *other* operand through `ToDecimal`. So whenever a `*Decimal` is the **left** operand (including `dyn` variables that hold a `*Decimal` at runtime), the singleton dispatches to *our* trait method and coercion succeeds. The failure only ever appeared when a **builtin `Int` sat on the left** with a non-single overload set.

### 3.3 The fix: make operands concrete, not `dyn`

We remove the ambiguity at its source (`getEnv`): **numeric inputs are declared as `DecimalType`, not `DynType`.** Then `size(x) / days` type-checks as `[int, decimal]`, which matches exactly one overload (`divide_int_decimal`) → the checker records the id → our binding runs. No singleton fallback, no left-operand dependency.

This is why the exa-specific signatures exist. In `lib.go`:

- **`CompileOptions()`** declares the signatures: `<op>_decimal_dyn` (`[Decimal, dyn]`) and `<op>_<T>_decimal` (`[T, Decimal]`) for every builtin `T`.
- **`ProgramOptions()`** registers the runtime bindings for those ids via `cel.Functions`, each coercing both sides with `ToDecimal`.

> **You cannot override the singleton.** `dispatcher.Add` rejects a second registration under an operator name (`_/_`), and the declaration layer rejects redefining a function's singleton binding. So the *only* levers you have are (a) **operand types** and (b) **per-overload-id bindings**. Attaching bindings alone never fixes a `dyn`-induced ambiguity — the type change in §3.3 is what does.

### 3.4 The one deliberate hijack: division

Builtin overload ids (`add_int64`, `divide_int64`, …) have **no** singleton dispatcher entry, so — unlike `_/_` — they *can* be registered. We exploit this in exactly one place: `divide_int64` / `divide_uint64` are overridden so that pure integer division (`size(a) / size(b)`) performs **high-precision decimal division** instead of truncating. Add/subtract/multiply/compare are **not** hijacked — their builtin results are numerically correct and simply coerced to `Decimal` at collection time (§4).

### 3.5 Keep the full overload matrix

`CompileOptions` declares `<op>_<T>_decimal` for *all* builtin `T` (string, list, timestamp, …), not just the numeric ones. The non-numeric pairings look dead but are **load-bearing**: they participate in the checker's overload resolution when an operand is `dyn` (e.g. a map/list index). Removing them makes the checker fail with `unexpected unspecified type` on dynamic-map expressions. If you touch `lhsTypes`, keep `CompileOptions.typeNames` and `ProgramOptions.typeNames` in sync and run the full suite.

---

## 4. Result Collection

Each rule's result is routed by its **runtime** type (`kernel.go`):

| Runtime value | Bucket |
|---|---|
| `*Decimal` | `Result.Decimals` |
| `types.Int` / `types.Uint` / `types.Double` | `Result.Decimals` (coerced) |
| `types.String` | `Result.Strings` |
| `types.Bool` | `Result.Bools` |

Coercing builtin numerics to `Decimal` here is what lets us *not* hijack add/sub/mul (§3.4) while still surfacing every numeric result. Preserving strings/bools means bare fact passthrough (e.g. a rule that just references `"M"`) is emitted directly — no downstream re-injection.

---

## 5. Extension Protocols

### Protocol A — Custom (non-operator) functions
- Declare parameters as `cel.DynType` so any input shape is accepted.
- Inside the binding, run numeric operands through `ToDecimal(val)` (returns a `ref.Val` error you must propagate).
- Return results via `NewDecimal(...)`.

```go
cel.Function("my_func",
    cel.Overload("my_func_any", []*cel.Type{cel.DynType}, DecimalType,
        cel.UnaryBinding(func(v ref.Val) ref.Val {
            d, err := ToDecimal(v)
            if err != nil { return err }
            return NewDecimal(d.Abs())
        }),
    ),
)
```

### Protocol B — Operators
Extend both halves in lock-step (see §3):
1. Add the signature(s) in `CompileOptions()` — declaration only, no binding.
2. Register the matching binding(s) in `ProgramOptions()`, coercing both sides via `ToDecimal`.
3. Do **not** hijack `add_/subtract_/multiply_/<cmp>_` builtins — rely on output coercion. Only division overrides builtins, and only to avoid integer truncation.

### Protocol C — Integer-returning builtins
- Functions that return structural integers (`size`, `year`, `month`, …) keep their native `int` type. Do not force them to `Decimal` at the source — it would break anything that needs a real int (see limitations).
- Their promotion into `Decimal` is handled automatically: by the `<op>_int_decimal` overloads when used in arithmetic, and by output coercion when returned directly.

---

## 6. Known Limitations

- **Computed list indices.** `list[size(list) - 1]` fails: literals are promoted to `Decimal` (stage 4), so `size(list) - 1` is a `Decimal` and CEL rejects a non-integer index. Index with integer literals directly, or restructure.
- **Numeric strings are numbers.** `"1"` is ingested as `Decimal(1)`; a numeric string cannot be preserved as text.
- **Policy references stay `dyn`.** Cross-rule references are `DynType`. `size(x) / someRuleId` therefore relies on the singleton path; it works because rule results are `*Decimal` at runtime, but a *builtin-Int LHS ÷ policy-ref* is the one shape still exercising §3.1's fallback.

---

## 7. Testing & Standards

```bash
go test ./pkg/exa          # full suite
go test -run TestFix ./pkg/exa   # bare-scalar / passthrough regression set
go vet ./...
```

Add tests next to the closest existing file:
- `engine_test.go` — core flows and builtins
- `robustness_test.go` — mixed-type boundaries, precision edges, DAG/error cases
- `korean_test.go` — internationalized identifiers & transpilation
- `handoff_fix_test.go` — operator resolution / result-partitioning regressions

Standards: idiomatic `go fmt`; keep comments explaining the *why* (especially around overload resolution — it is easy to "simplify" into a regression); every numeric path must remain `shopspring/decimal`-backed.
