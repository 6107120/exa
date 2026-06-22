# Exa: High-Precision Calculation Kernel

[![Go Report Card](https://goreportcard.com/badge/github.com/6107120/exa)](https://goreportcard.com/report/github.com/6107120/exa)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Exa** is a high-performance, high-precision calculation kernel built on top of Google's Common Expression Language (CEL). It is specifically designed for complex business logic, financial settlements, and payroll systems where mathematical integrity is non-negotiable.

---

## 💎 Core Philosophy

- **Mathematical Integrity:** All numeric operations are automatically promoted to arbitrary-precision `decimal.Decimal`. No floating-point errors (`0.1 + 0.2 == 0.3`).
- **Dense & Elegant:** Logic is expressed through formulas (expressions), eliminating unnecessary boilerplate and deep nesting.
- **Auto-Dependency Resolution:** Built-in DAG (Directed Acyclic Graph) analysis automatically determines the execution order of your formulas.
- **Zero-Config Performance:** Transparent execution plan (Program) caching ensures repetitive calculations are lightning-fast.

---

## 📦 Installation

```bash
go get github.com/6107120/exa
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
			"base_salary": "5000.00",
			"bonus_rate":  0.15,
		},
		Policy: []exa.Calculation{
			{ID: "tax_rate", Expression: "0.033"},
			{ID: "bonus",    Expression: "base_salary * bonus_rate"},
			{ID: "total",    Expression: "(base_salary + bonus) * (1 - tax_rate)"},
		},
	}

	// Computes in order: tax_rate -> bonus -> total
	res, err := exa.Compute(req) 
	if err != nil {
		panic(err)
	}

	fmt.Printf("Total Result: %s\n", res["total"]) // Total Result: 5559.75
}
```

---

## 🛠 Features & API

### 1. High-Precision Ingestion
To maintain financial-grade integrity, use the following input types:
- **String:** `"10.50"` (Most Recommended)
- **json.Number:** Preserves precision during API integration.
- **Float64:** Automatically converted to Decimal, but be wary of IEEE 754 precision limits at the source.

### 2. Vector Primitives
High-performance array operations for processing large datasets.
- `vec_add(L1, L2)`, `vec_sub(L1, L2)`, `vec_mul(L1, L2)`, `vec_div(L1, L2)`
- `vec_scale(scalar, list)`: Scalar-Vector multiplication.
- `vec_sum(list)`: Sum of all elements in an array.

### 3. Temporal Primitives
Precise date and time operations aligned with business logic.
- **Extraction:** `year(d)`, `month(d)`, `day(d)`, `hour(d)`, `minute(d)`, `second(d)`
- **Duration:** `days_between(start, end)` (Inclusive calculation).
- **Metadata:** `days_in_month(d)` (Returns actual number of days in the month).

**Pro-rata Example:**
```go
// Transparent pro-rata calculation based on actual days in the month
"days_between(start, end) / days_in_month(start)"
```

### 4. Constraints & Validation
- **`assert(condition, message)`:** Enforce business rules within expressions. Returns a custom error on violation.

---

## ⚡ Performance Optimization

For high-load environments (e.g., batch processing), reuse the `Engine` instance to benefit from internal caching.
```go
engine := exa.NewEngine()
for {
    // Compiled CEL programs are cached automatically based on expression and environment signature.
    res, err := engine.Compute(ctx, req) 
}
```

---

## ⚖️ License

Distributed under the MIT License. See `LICENSE` for more information.
