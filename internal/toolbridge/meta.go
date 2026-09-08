package toolbridge

import (
	"context"
	"encoding/json"
)

type ListToolsInput struct {
	Skill string `json:"skill,omitempty"`
}

type ToolDescriptor struct {
	Skill       string `json:"skill"`
	Tool        string `json:"tool"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type ListToolsOutput struct {
	Tools []ToolDescriptor `json:"tools"`
}

// RegisterMeta makes the startup-frozen proxy catalog discoverable with the
// same schema data that dispatch uses.
func (r *Registry) RegisterMeta() {
	if err := r.Add(SkillMeta, "list_tools", Bound{Call: func(_ context.Context, raw json.RawMessage) (any, error) {
		var in ListToolsInput
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
		}
		out := ListToolsOutput{Tools: []ToolDescriptor{}}
		for _, op := range r.List() {
			if in.Skill != "" && in.Skill != op.Skill {
				continue
			}
			out.Tools = append(out.Tools, ToolDescriptor{Skill: op.Skill, Tool: op.Tool, InputSchema: op.RawInputSchema})
		}
		return out, nil
	}}); err != nil {
		panic(err)
	}
}
