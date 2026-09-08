package a2aserver

import "encoding/json"

// Request is the JSON envelope a caller sends as the task message text.
//
// The request names a skill, a tool, and tool arguments. Args use exactly the
// input schema synchronized from the private MCP. Intent is an optional
// natural-language, read-only Lendora request.
type Request struct {
	Skill  string          `json:"skill"`
	Tool   string          `json:"tool,omitempty"`
	Args   json.RawMessage `json:"args,omitempty"`
	Caller string          `json:"caller,omitempty"`
	Intent string          `json:"intent,omitempty"`
}

// Response is the agent's reply, serialized as the message text.
//
// A refused or failed operation is a normal response with OK=false — the task
// ran and produced an answer, and that answer is "no". A caller distinguishes
// this from a transport error.
type Response struct {
	Skill  string `json:"skill"`
	Tool   string `json:"tool,omitempty"`
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}
