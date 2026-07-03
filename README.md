# Exa: High-Precision Calculation Kernel

[![Go Report Card](https://goreportcard.com/badge/github.com/6107120/exa)](https://goreportcard.com/report/github.com/6107120/exa)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Exa** is a high-performance, high-precision calculation kernel built on top of Google's Common Expression Language (CEL). It is designed for complex business logic — financial settlements, payroll, attendance — where mathematical integrity is non-negotiable.

You feed it **plain values** and **plain formulas**; Exa handles precision, type unification, dependency ordering, and caching for you.

---

## 💎 Core Philosophy

- **Mathematical Integrity** — every numeric value flows through arbitrary-precision `decimal.Decimal`. No IEEE-754 drift (`0.1 + 0.2 == 0.3`).
- **No Explicit Casting** — write `size(segs) / days`, not `decimal(size(segs)) / decimal(days)`. Type handling is fully internal; you never annotate types or wrap operands. (See [Type Model](#-type-model).)
- **Dense & Elegant** — logic lives in formulas, not boilerplate. A batch is just a list of `{id, expression}`.
- **Auto-Dependency Resolution** — a DAG analysis derives execution order from the expressions themselves.
- **Zero-Config Performance** — compiled CEL programs and environments are cached transparently.

---

## 📦 Installation

```bash
go get github.com/6107120/exa@latest
```

---

## 🚀 Quick Start

```go
package main

import (
	"context"
	"fmt"

	"github.com/6107120/exa/pkg/exa"
)

func main() {
	req := exa.Request{
		Inputs: map[string]any{
			"base_salary": "5000.00", // string preserves precision
			"bonus_rate":  0.15,
		},
		Policy: []exa.Calculation{
			{ID: "tax_rate", Expression: "0.033"},
			{ID: "bonus", Expression: "base_salary * bonus_rate"},
			{ID: "total", Expression: "(base_salary + bonus) * (1 - tax_rate)"},
		},
	}

	// Executes in dependency order: tax_rate -> bonus -> total
	res, err := exa.Compute(req)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Total: %s\n", res.Decimals["total"]) // Total: 5560.25
}
```

---

## 📤 The `Result`

`Compute` returns a single `Result` carrying every rule's output, partitioned by runtime type:

```go
type Result struct {
	Decimals map[string]decimal.Decimal // numeric results (all numbers land here)
	Strings  map[string]string          // string results / bare fact passthrough
	Bools    map[string]bool            // comparison results, boolean facts
}
```

Any numeric CEL value (including bare `int` results such as `size(x)`) is coerced to `Decimal` and placed in `Decimals`. String and bool results are preserved rather than dropped, so **passthrough rules require no downstream re-injection**:

```go
req := exa.Request{
	Inputs: map[string]any{"unit": "M", "minutes": 480},
	Policy: []exa.Calculation{
		{ID: "settlement_unit", Expression: "unit"},    // -> Strings["settlement_unit"] = "M"
		{ID: "daily", Expression: "minutes"},            // -> Decimals["daily"] = 480
		{ID: "is_full", Expression: "minutes >= 480"},   // -> Bools["is_full"] = true
	},
}
```

---

## 🧬 Type Model

- **Inputs** — numeric values (`int`, `float`, numeric strings like `"10.50"`, `json.Number`) are treated as high-precision `Decimal`; non-numeric values (`"M"`, lists, maps) stay dynamic.
- **Literals** — numeric literals in expressions are automatically promoted to `Decimal`.
- **Builtins** — functions that return structural integers (`size`, `year`, …) keep their native `int` type and are coerced to `Decimal` only when they surface as a result or enter arithmetic.

> ⚠️ **`"1"` vs `1`** — a numeric-looking string is treated as the *number* `1`, not the string `"1"`. Exa cannot preserve a numeric string as text; use a non-numeric marker if you need a string label.

---

## 🛠 Features & API

### High-Precision Ingestion
| Input type | Notes |
|---|---|
| `string` | **Recommended** — e.g. `"10.50"`, exact. |
| `json.Number` | Preserves precision across API/JSON boundaries. |
| `float64` | Converted to Decimal; mind IEEE-754 limits *at the source*. |

### Vector Primitives
High-performance elementwise array operations.
- `vec_add(L1, L2)`, `vec_sub(L1, L2)`, `vec_mul(L1, L2)`, `vec_div(L1, L2)`
- `vec_scale(scalar, list)` — scalar × vector
- `vec_sum(list)` — sum of all elements

### Temporal Primitives
- **Extraction:** `year(d)`, `month(d)`, `day(d)`, `hour(d)`, `minute(d)`, `second(d)`
- **Duration:** `days_between(start, end)` (inclusive)
- **Metadata:** `days_in_month(d)`

```go
// Transparent pro-rata based on actual days in the month
"days_between(start, end) / days_in_month(start)"
```

### Math Helpers
`abs`, `min`, `max`, `pow`, `round`, `round_down`, `ceil`, `sum`, `decimal`.

### Constraints & Validation
- `assert(condition, message)` — enforce a business rule inside an expression; returns a custom error on violation.

### Internationalized Identifiers
Non-ASCII identifiers (e.g. `근무유형`, `税金`) are supported: they are Unicode-normalized (NFC) and transparently transpiled, then decoded back to their original form in the `Result`.

---

## ⚡ Performance

Reuse a single `Engine` in high-load / batch paths to benefit from environment and program caching:

```go
engine := exa.NewEngine()
for _, req := range batch {
	// Programs are cached by expression + environment signature.
	res, err := engine.Compute(ctx, req)
	_ = res; _ = err
}
```

---

## 🤝 Contributing

Adding a function or operator? Read **[CONTRIBUTING.md](CONTRIBUTING.md)** — it documents the execution pipeline and, critically, the CEL overload-resolution model you must understand before touching operators.

---

## ⚖️ License

Distributed under the MIT License. See `LICENSE`.
