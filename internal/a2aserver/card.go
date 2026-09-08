package a2aserver

import (
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
)

type skillMeta struct {
	id   string
	name string
	desc string
	tags []string
}

// The public Lending Agent has exactly three skills: catalog discovery,
// token-authorized Lendora tools forwarded to DeFi MCP, and local EVM landing.
var skillMetas = []skillMeta{
	{
		id:   toolbridge.SkillMeta,
		name: "SVP-Chain Agent Self-Description",
		desc: "Discovery of the startup-synchronized tool catalog and argument schemas.",
		tags: []string{"discovery", "schema", "read-only"},
	},
	{
		id:   toolbridge.SkillLendora,
		name: "Lendora Money Market",
		desc: "Lendora tools synchronized from the private DeFi MCP at startup.",
		tags: []string{"lendora", "lending", "money-market", "mcp"},
	},
	{
		id:   toolbridge.SkillEVM,
		name: "SVP-Chain EVM",
		desc: "Caller-signed EVM transaction broadcast and transaction-status lookup.",
		tags: []string{"evm", "broadcast", "tx-status"},
	},
}

// CardIdentity is the per-binary half of the Agent Card.
type CardIdentity struct {
	Name        string
	Version     string
	Description string

	SkillDescOverrides map[string]string
}

// BuildAgentCardFor returns the public Agent Card. The registry supplies each
// tool list, so the card cannot advertise an operation the executor lacks.
func BuildAgentCardFor(id CardIdentity, publicURL string, reg *toolbridge.Registry) *a2a.AgentCard {
	bySkill := map[string][]string{}
	if reg != nil {
		bySkill = reg.BySkill()
	}

	var skills []a2a.AgentSkill
	for _, m := range skillMetas {
		tools := bySkill[m.id]
		if len(tools) == 0 {
			continue
		}
		desc := m.desc
		if o, ok := id.SkillDescOverrides[m.id]; ok {
			desc = o
		}
		desc = fmt.Sprintf("%s Tools: %s.", desc, strings.Join(tools, ", "))
		skills = append(skills, a2a.AgentSkill{
			ID:          m.id,
			Name:        m.name,
			Description: desc,
			Tags:        m.tags,
		})
	}

	return &a2a.AgentCard{
		Name:        id.Name,
		Description: id.Description,
		Version:     id.Version,
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(publicURL+"/invoke", a2a.TransportProtocolJSONRPC),
		},
		DefaultInputModes:  []string{"application/json", "text/plain"},
		DefaultOutputModes: []string{"application/json"},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		Provider: &a2a.AgentProvider{
			Org: "svpchain",
			URL: "https://www.svpchain.org",
		},
		Skills: skills,
	}
}
