# Exa: High-Precision Calculation Kernel

[![Go Report Card](https://goreportcard.com/badge/github.com/6107120/exa)](https://goreportcard.com/report/github.com/6107120/exa)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## 💎 Mathematical Integrity & Precision

Exa is built to eliminate the common pitfalls of binary floating-point arithmetic. 

- **Internal Representation:** All numbers are converted to `decimal.Decimal` (arbitrary-precision) before any calculation begins.
- **Recommended Inputs:** For maximum integrity (especially in financial apps), we recommend passing numbers as **Strings** or using **`json.Number`**.
- **Float Warning:** If you pass a `float64`, it is already subject to IEEE 754 precision limits (~15-17 digits) before Exa even receives it. Use Strings to preserve 20+ digits of precision.

```go
inputs := map[string]any{
    "precise_val": "3.33333333333333333333333333", // Recommended
    "json_val": json.Number("10.50"),             // Excellent for API integration
    "legacy_val": 10.5,                          // Safe for simple decimals
}
```

```bash
go get github.com/6107120/exa
```

## 💡 Quick Start

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
			"base": 5000,
			"rate": "0.15",
		},
		Policy: []exa.Calculation{
			{ID: "tax", Expression: "base * rate"},
			{ID: "total", Expression: "base + tax"},
		},
	}

	res, err := exa.Compute(req)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Total: %s\n", res["total"]) // Total: 5750
}
```

## 🛠 Advanced Features

### Dynamic Assertions
Enforce business constraints directly within your expressions.

```go
{
    ID: "validate", 
    Expression: "assert(balance >= 0, 'Insufficient funds')"
}
```

### Built-in Functions
- `abs(x)`, `min(a, b)`, `max(a, b)`
- `sum(list)`
- `round(x, precision)`, `ceil(x, precision)`, `round_down(x, precision)`
- `pow(base, exp)`

## ⚖️ License

Distributed under the MIT License. See `LICENSE` for more information.

---
Built with ❤️ for the Go Open Source Community.
