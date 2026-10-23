package core

// MachineLook is how a stage's machine looks: an agent stage's comes from its agent, the
// others' from what they do.
type MachineLook struct {
	Machine Machine `json:"machine"`
	Tint    Tint    `json:"tint"`
}

// MachineLookOf is the machine and tint a stage is drawn with; agent is the stage's agent, nil when it has none.
func MachineLookOf(stage Stage, agent *Agent) MachineLook {
	switch stage.Type {
	case StageTypeAgent:
		look := MachineLook{Machine: MachineBox, Tint: TintGrey}
		if agent != nil {
			look.Machine = agent.Machine
			look.Tint = agent.Tint
		}
		return look
	case StageTypeApprove:
		return MachineLook{Machine: MachineMailbox, Tint: TintGold}
	default:
		return MachineLook{Machine: MachineReader, Tint: TintSand}
	}
}
