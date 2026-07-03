package exa

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/shopspring/decimal"
)

// Engine is the core high-precision expression kernel.
type Engine struct {
	envCache     sync.Map // map[string]*cel.Env
	programCache sync.Map // map[string]cel.Program
}

func NewEngine() *Engine {
	return &Engine{}
}

// Compute executes a set of formulas against a given input context and returns
// the full Result, including non-numeric (string/bool) results.
func (e *Engine) Compute(ctx context.Context, req Request) (Result, error) {
	// 0. Preprocessing: Normalize and clean all Unicode strings (NFC, remove invisible control chars, etc.)
	req = NormalizeRequest(req)

	res, err := e.compute(ctx, req)
	if err != nil {
		return Result{}, DeobfuscateError(err)
	}
	return res, nil
}

func (e *Engine) compute(ctx context.Context, req Request) (Result, error) {
	// 1. High-performance Fast-Path: If no Unicode characters are present, bypass transpilation completely.
	var transpiledReq Request
	isTranspiled := NeedsTranspilation(req)
	if isTranspiled {
		transpiledReq = TranspileRequest(req)
	} else {
		transpiledReq = req
	}

	// 2. Validate identifiers and compile ASTs
	uniqueKeys := make(map[string]bool)
	for k := range transpiledReq.Inputs {
		if uniqueKeys[k] { return Result{}, fmt.Errorf("%w: %s", ErrDuplicateID, k) }
		uniqueKeys[k] = true
	}
	for _, p := range transpiledReq.Policy {
		if uniqueKeys[p.ID] { return Result{}, fmt.Errorf("%w: %s", ErrDuplicateID, p.ID) }
		uniqueKeys[p.ID] = true
	}

	env, err := e.getEnv(transpiledReq)
	if err != nil {
		return Result{}, err
	}

	// 3. Sort nodes by dependency (DAG)
	nodes, err := sortByDependencies(transpiledReq.Policy)
	if err != nil {
		return Result{}, err
	}

	// 4. Execution Context (Flat namespace)
	results := make(map[string]any)
	for k, v := range transpiledReq.Inputs {
		results[k] = convertToRefVal(v)
	}

	// 5. Sequential Execution
	rs := Result{
		Decimals: make(map[string]decimal.Decimal),
		Strings:  make(map[string]string),
		Bools:    make(map[string]bool),
	}
	for _, node := range nodes {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}

		// Cache key: [envCacheKey]:[expression]
		progKey := fmt.Sprintf("%s:%s", e.getEnvKey(transpiledReq), node.Expression)
		var prg cel.Program
		if val, ok := e.programCache.Load(progKey); ok {
			prg = val.(cel.Program)
		} else {
			var err error
			prg, err = env.Program(node.ast)
			if err != nil {
				return Result{}, &EvalError{ID: node.ID, Inner: err}
			}
			e.programCache.Store(progKey, prg)
		}

		res, _, err := prg.Eval(results)
		if err != nil {
			return Result{}, &EvalError{ID: node.ID, Inner: err}
		}

		results[node.ID] = res

		// Collect the result, coercing any numeric CEL value to Decimal so that
		// integer-typed results (e.g. size(x)) still surface, and capturing
		// string/bool results that would otherwise be dropped.
		key := decodeUnicodeIdent(node.ID)
		switch v := res.(type) {
		case *Decimal:
			rs.Decimals[key] = v.Decimal
		case types.Int:
			rs.Decimals[key] = decimal.NewFromInt(int64(v))
		case types.Uint:
			rs.Decimals[key] = decimal.NewFromUint64(uint64(v))
		case types.Double:
			rs.Decimals[key] = decimal.NewFromFloat(float64(v))
		case types.String:
			rs.Strings[key] = string(v)
		case types.Bool:
			rs.Bools[key] = bool(v)
		}
	}

	return rs, nil
}

func (e *Engine) getEnvKey(req Request) string {
	keys := make([]string, 0, len(req.Inputs)+len(req.Policy))
	// Inputs carry a type code because numeric inputs are declared with the concrete
	// DecimalType (not DynType); the same name may map to different types across calls.
	for k, v := range req.Inputs { keys = append(keys, k+"#"+inputTypeCode(v)) }
	for _, p := range req.Policy { keys = append(keys, p.ID) }
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// isNumericInput reports whether a raw input value will be converted to a Decimal
// by the registry (mirrors Registry.NativeToValue), so it can be declared with a
// concrete DecimalType instead of DynType.
func isNumericInput(v any) bool {
	switch x := v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	case json.Number:
		_, err := decimal.NewFromString(string(x))
		return err == nil
	case string:
		_, err := decimal.NewFromString(x)
		return err == nil
	default:
		return false
	}
}

func inputTypeCode(v any) string {
	if isNumericInput(v) {
		return "dec"
	}
	return "dyn"
}

func (e *Engine) getEnv(req Request) (*cel.Env, error) {
	cacheKey := e.getEnvKey(req)
	if val, ok := e.envCache.Load(cacheKey); ok {
		return val.(*cel.Env), nil
	}

	reg := NewRegistry()
	opts := []cel.EnvOption{
		cel.Lib(Library{}),
		cel.Macros(cel.StandardMacros...),
		cel.CustomTypeAdapter(reg),
	}
	// Numeric inputs are declared with the concrete DecimalType so the checker
	// resolves a single arithmetic/comparison overload even when the other operand
	// is a builtin Int (e.g. size(x)); non-numeric inputs and all policy results
	// stay DynType. This is what makes flat "size(segs) / days" resolve cleanly.
	for k, v := range req.Inputs {
		if isNumericInput(v) {
			opts = append(opts, cel.Variable(k, DecimalType))
		} else {
			opts = append(opts, cel.Variable(k, cel.DynType))
		}
	}
	for _, p := range req.Policy {
		opts = append(opts, cel.Variable(p.ID, cel.DynType))
	}

	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create env: %w", err)
	}

	// Compile all ASTs and store them in the policy objects for this request
	// (Note: In a more optimized version, we'd cache the ASTs too)
	optimizer := &literalOptimizer{}
	for i := range req.Policy {
		p := &req.Policy[i]
		parsed, issues := env.Parse(p.Expression)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("parse error for [%s]: %w", p.ID, issues.Err())
		}
		
		optimized, _ := cel.NewStaticOptimizer(optimizer)
		optAST, _ := optimized.Optimize(env, parsed)
		
		checked, issues := env.Check(optAST)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("check error for [%s]: %w", p.ID, issues.Err())
		}
		p.ast = checked
	}

	e.envCache.Store(cacheKey, env)
	return env, nil
}

// --- Internal Helper: DAG Sorting ---

func sortByDependencies(policy []Calculation) ([]*Calculation, error) {
	// 1. Build dependency graph by scanning identifiers in AST
	// (Assuming AST is already populated in getEnv)
	visited := make(map[string]int) // 0: unvisited, 1: visiting, 2: visited
	var sorted []*Calculation
	var dfs func(id string) error

	dfs = func(id string) error {
		state := visited[id]
		if state == 1 { return fmt.Errorf("%w at %s", ErrCircularDependency, id) }
		if state == 2 { return nil }

		visited[id] = 1
		node, ok := findNode(policy, id)
		if ok {
			deps := extractDeps(node.ast)
			for _, dep := range deps {
				if _, ok := findNode(policy, dep); ok {
					if err := dfs(dep); err != nil { return err }
				}
			}
		}

		visited[id] = 2
		if ok { sorted = append(sorted, node) }
		return nil
	}

	for _, p := range policy {
		if err := dfs(p.ID); err != nil { return nil, err }
	}
	return sorted, nil
}

func findNode(policy []Calculation, id string) (*Calculation, bool) {
	for i := range policy {
		if policy[i].ID == id { return &policy[i], true }
	}
	return nil, false
}

func extractDeps(a *cel.Ast) []string {
	if a == nil { return nil }
	deps := make(map[string]bool)
	ast.PreOrderVisit(a.NativeRep().Expr(), ast.NewExprVisitor(func(e ast.Expr) {
		if e.Kind() == ast.IdentKind {
			deps[e.AsIdent()] = true
		}
	}))
	var res []string
	for d := range deps { res = append(res, d) }
	return res
}

// --- Internal Helper: Literal Optimizer ---

type literalOptimizer struct{}

func (o *literalOptimizer) Optimize(ctx *cel.OptimizerContext, a *ast.AST) *ast.AST {
	fac := ast.NewExprFactory()
	maxID := int64(0)
	ast.PreOrderVisit(a.Expr(), ast.NewExprVisitor(func(e ast.Expr) {
		if e.ID() > maxID { maxID = e.ID() }
	}))
	nextID := func() int64 { maxID++; return maxID }
	return ast.NewAST(o.rewrite(fac, nextID, a.Expr()), a.SourceInfo())
}

func (o *literalOptimizer) rewrite(fac ast.ExprFactory, nextID func() int64, e ast.Expr) ast.Expr {
	if e == nil { return nil }
	switch e.Kind() {
	case ast.LiteralKind:
		if v, ok := e.AsLiteral().Value().(float64); ok {
			arg := fac.NewLiteral(nextID(), types.String(strconv.FormatFloat(v, 'f', -1, 64)))
			return fac.NewCall(nextID(), "decimal", arg)
		}
		if v, ok := e.AsLiteral().Value().(int64); ok {
			arg := fac.NewLiteral(nextID(), types.String(strconv.FormatInt(v, 10)))
			return fac.NewCall(nextID(), "decimal", arg)
		}
	case ast.CallKind:
		call := e.AsCall()
		args := make([]ast.Expr, len(call.Args()))
		for i, a := range call.Args() {
			args[i] = o.rewrite(fac, nextID, a)
		}
		if call.IsMemberFunction() {
			return fac.NewMemberCall(e.ID(), call.FunctionName(), o.rewrite(fac, nextID, call.Target()), args...)
		}
		return fac.NewCall(e.ID(), call.FunctionName(), args...)
	case ast.SelectKind:
		sel := e.AsSelect()
		operand := o.rewrite(fac, nextID, sel.Operand())
		if sel.IsTestOnly() {
			return fac.NewPresenceTest(e.ID(), operand, sel.FieldName())
		}
		return fac.NewSelect(e.ID(), operand, sel.FieldName())
	case ast.ListKind:
		list := e.AsList()
		elems := make([]ast.Expr, len(list.Elements()))
		for i, el := range list.Elements() {
			elems[i] = o.rewrite(fac, nextID, el)
		}
		return fac.NewList(e.ID(), elems, list.OptionalIndices())
	case ast.MapKind:
		m := e.AsMap()
		entries := make([]ast.EntryExpr, len(m.Entries()))
		for i, entry := range m.Entries() {
			if entry.Kind() == ast.MapEntryKind {
				me := entry.AsMapEntry()
				k := o.rewrite(fac, nextID, me.Key())
				v := o.rewrite(fac, nextID, me.Value())
				entries[i] = fac.NewMapEntry(entry.ID(), k, v, me.IsOptional())
			} else {
				entries[i] = entry
			}
		}
		return fac.NewMap(e.ID(), entries)
	case ast.StructKind:
		str := e.AsStruct()
		fields := make([]ast.EntryExpr, len(str.Fields()))
		for i, entry := range str.Fields() {
			if entry.Kind() == ast.StructFieldKind {
				sf := entry.AsStructField()
				v := o.rewrite(fac, nextID, sf.Value())
				fields[i] = fac.NewStructField(entry.ID(), sf.Name(), v, sf.IsOptional())
			} else {
				fields[i] = entry
			}
		}
		return fac.NewStruct(e.ID(), str.TypeName(), fields)
	case ast.ComprehensionKind:
		comp := e.AsComprehension()
		iterRange := o.rewrite(fac, nextID, comp.IterRange())
		accuInit := o.rewrite(fac, nextID, comp.AccuInit())
		loopCondition := o.rewrite(fac, nextID, comp.LoopCondition())
		loopStep := o.rewrite(fac, nextID, comp.LoopStep())
		result := o.rewrite(fac, nextID, comp.Result())
		if comp.HasIterVar2() {
			return fac.NewComprehensionTwoVar(e.ID(),
				iterRange,
				comp.IterVar(),
				comp.IterVar2(),
				comp.AccuVar(),
				accuInit,
				loopCondition,
				loopStep,
				result,
			)
		}
		return fac.NewComprehension(e.ID(),
			iterRange,
			comp.IterVar(),
			comp.AccuVar(),
			accuInit,
			loopCondition,
			loopStep,
			result,
		)
	}
	return e
}

func convertToRefVal(v any) ref.Val {
	return NewRegistry().NativeToValue(v)
}
