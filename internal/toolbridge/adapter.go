// Package toolbridge exposes the private DeFi MCP catalog and the small local
// EVM landing rail through the public A2A service.
package toolbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"
)

// Op is one invokable operation bound to an advertised A2A skill.
type Op struct {
	Skill          string
	Tool           string
	Call           func(context.Context, json.RawMessage) (any, error)
	RawInputSchema any
}

// Bound is a typed local handler and its input schema.
type Bound struct {
	Call        func(context.Context, json.RawMessage) (any, error)
	InputSchema *jsonschema.Schema
}

func schemaFor[In any]() *jsonschema.Schema {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		return nil
	}
	return schema
}

// Native exposes a local handler. The only local tools are signed EVM
// broadcast and transaction status; DeFi operations are always proxied.
func Native[In, Out any](f func(context.Context, In) (Out, error)) Bound {
	return Bound{InputSchema: schemaFor[In](), Call: adaptNative(f)}
}

func adaptNative[In, Out any](f func(context.Context, In) (Out, error)) func(context.Context, json.RawMessage) (any, error) {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in In
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, fmt.Errorf("decode args: %w", err)
			}
		}
		return f(ctx, in)
	}
}

// Registry maps tool names to operations and groups them by skill for the
// Agent Card.
type Registry struct {
	ops map[string]Op
}

func newRegistry() *Registry { return &Registry{ops: map[string]Op{}} }

// AddProxy exposes one startup-synchronized private MCP tool without copying
// its implementation into this agent.
func (r *Registry) AddProxy(skill, tool string, schema any, call func(context.Context, map[string]any) (string, error)) error {
	if _, dup := r.ops[tool]; dup {
		return fmt.Errorf("toolbridge: duplicate tool %q", tool)
	}
	r.ops[tool] = Op{Skill: skill, Tool: tool, RawInputSchema: schema, Call: func(ctx context.Context, raw json.RawMessage) (any, error) {
		args := map[string]any{}
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &args); err != nil {
				return nil, fmt.Errorf("decode args: %w", err)
			}
			if args == nil {
				return nil, fmt.Errorf("args must be an object")
			}
		}
		return call(ctx, args)
	}}
	return nil
}

func (r *Registry) Add(skill, tool string, bound Bound) error {
	if _, dup := r.ops[tool]; dup {
		return fmt.Errorf("toolbridge: duplicate tool %q", tool)
	}
	r.ops[tool] = Op{Skill: skill, Tool: tool, Call: bound.Call, RawInputSchema: bound.InputSchema}
	return nil
}

// Lookup returns the operation registered under tool.
func (r *Registry) Lookup(tool string) (Op, bool) {
	op, ok := r.ops[tool]
	return op, ok
}

func (r *Registry) List() []Op {
	out := make([]Op, 0, len(r.ops))
	for _, op := range r.ops {
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tool < out[j].Tool })
	return out
}

// BySkill returns tool names grouped by skill, each group sorted.
func (r *Registry) BySkill() map[string][]string {
	out := map[string][]string{}
	for _, op := range r.ops {
		out[op.Skill] = append(out[op.Skill], op.Tool)
	}
	for _, tools := range out {
		sort.Strings(tools)
	}
	return out
}
