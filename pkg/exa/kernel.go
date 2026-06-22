package exa

import (
	"context"
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

// Compute executes a set of formulas against a given input context.
func (e *Engine) Compute(ctx context.Context, req Request) (map[string]decimal.Decimal, error) {
	// 1. Validate identifiers and compile ASTs
	uniqueKeys := make(map[string]bool)
	for k := range req.Inputs {
		if uniqueKeys[k] { return nil, fmt.Errorf("%w: %s", ErrDuplicateID, k) }
		uniqueKeys[k] = true
	}
	for _, p := range req.Policy {
		if uniqueKeys[p.ID] { return nil, fmt.Errorf("%w: %s", ErrDuplicateID, p.ID) }
		uniqueKeys[p.ID] = true
	}

	env, err := e.getEnv(req)
	if err != nil {
		return nil, err
	}

	// 2. Sort nodes by dependency (DAG)
	nodes, err := sortByDependencies(req.Policy)
	if err != nil {
		return nil, err
	}

	// 3. Execution Context (Flat namespace)
	results := make(map[string]any)
	for k, v := range req.Inputs {
		results[k] = convertToRefVal(v)
	}

	// 4. Sequential Execution
	output := make(map[string]decimal.Decimal)
	for _, node := range nodes {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Cache key: [envCacheKey]:[expression]
		progKey := fmt.Sprintf("%s:%s", e.getEnvKey(req), node.Expression)
		var prg cel.Program
		if val, ok := e.programCache.Load(progKey); ok {
			prg = val.(cel.Program)
		} else {
			var err error
			prg, err = env.Program(node.ast)
			if err != nil {
				return nil, &EvalError{ID: node.ID, Inner: err}
			}
			e.programCache.Store(progKey, prg)
		}

		res, _, err := prg.Eval(results)
		if err != nil {
			return nil, &EvalError{ID: node.ID, Inner: err}
		}

		results[node.ID] = res
		
		if d, ok := res.(*Decimal); ok {
			output[node.ID] = d.Decimal
		}
	}

	return output, nil
}

func (e *Engine) getEnvKey(req Request) string {
	keys := make([]string, 0, len(req.Inputs)+len(req.Policy))
	for k := range req.Inputs { keys = append(keys, k) }
	for _, p := range req.Policy { keys = append(keys, p.ID) }
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func (e *Engine) getEnv(req Request) (*cel.Env, error) {
	cacheKey := e.getEnvKey(req)
	if val, ok := e.envCache.Load(cacheKey); ok {
		return val.(*cel.Env), nil
	}

	keys := strings.Split(cacheKey, ",")
	reg := NewRegistry()
	opts := []cel.EnvOption{
		cel.Lib(Library{}),
		cel.Macros(cel.StandardMacros...),
		cel.CustomTypeAdapter(reg),
	}
	for _, k := range keys {
		if k == "" { continue }
		opts = append(opts, cel.Variable(k, cel.DynType))
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
		for i, a := range call.Args() { args[i] = o.rewrite(fac, nextID, a) }
		if call.IsMemberFunction() {
			return fac.NewMemberCall(e.ID(), call.FunctionName(), o.rewrite(fac, nextID, call.Target()), args...)
		}
		return fac.NewCall(e.ID(), call.FunctionName(), args...)
	}
	return e
}

func convertToRefVal(v any) ref.Val {
	return NewRegistry().NativeToValue(v)
}
