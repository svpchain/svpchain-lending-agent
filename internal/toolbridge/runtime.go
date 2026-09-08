package toolbridge

const (
	SkillMeta    = "svpchain-meta"
	SkillLendora = "svpchain-lendora"
	SkillEVM     = "svpchain-evm"
)

func NewEmpty() *Registry { return newRegistry() }
