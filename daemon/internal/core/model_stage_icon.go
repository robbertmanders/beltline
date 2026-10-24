package core

// StageIcon is the glyph a stage box shows. An agent picks one; approvals,
// watches and triggers have their own.
type StageIcon string

const (
	StageIconFunnel  StageIcon = "funnel"
	StageIconDoc     StageIcon = "doc"
	StageIconApprove StageIcon = "approve"
	StageIconWrench  StageIcon = "wrench"
	StageIconScope   StageIcon = "scope"
	StageIconShip    StageIcon = "ship"
	StageIconMerge   StageIcon = "merge"
	StageIconFlag    StageIcon = "flag"
	StageIconEye     StageIcon = "eye"
	StageIconSend    StageIcon = "send"
	StageIconList    StageIcon = "list"
	StageIconPen     StageIcon = "pen"
	StageIconCycle   StageIcon = "cycle"
	StageIconBranch  StageIcon = "branch"
	StageIconGauge   StageIcon = "gauge"
	StageIconChip    StageIcon = "chip"
	// The icons below are only for the stages and triggers that don't come from an agent.
	StageIconBolt  StageIcon = "bolt"
	StageIconJira  StageIcon = "jira"
	StageIconClock StageIcon = "clock"
	StageIconHand  StageIcon = "hand"
)

// AllStageIcon lists every StageIcon in declaration order (CaseIterable).
var AllStageIcon = []StageIcon{
	StageIconFunnel, StageIconDoc, StageIconApprove, StageIconWrench, StageIconScope, StageIconShip, StageIconMerge,
	StageIconFlag, StageIconEye, StageIconSend, StageIconList, StageIconPen, StageIconCycle, StageIconBranch,
	StageIconGauge, StageIconChip, StageIconBolt, StageIconJira, StageIconClock, StageIconHand,
}

// Known reports whether x is one of the cases of StageIcon.
func (x StageIcon) Known() bool { return enumKnown(x, AllStageIcon) }

// StageIconAgentChoices are the icons an agent may pick.
var StageIconAgentChoices = []StageIcon{
	StageIconFunnel, StageIconDoc, StageIconApprove, StageIconWrench, StageIconScope, StageIconShip, StageIconMerge,
	StageIconFlag, StageIconEye, StageIconSend, StageIconList, StageIconPen, StageIconCycle, StageIconBranch,
	StageIconGauge, StageIconChip,
}

// Label is the name shown for the icon: the raw value
// with its first letter upper-cased, except for the four named differently.
func (i StageIcon) Label() string {
	switch i {
	case StageIconDoc:
		return "Document"
	case StageIconApprove:
		return "Approval"
	case StageIconScope:
		return "Magnifier"
	case StageIconChip:
		return "Chip"
	}
	s := string(i)
	if s == "" {
		return ""
	}
	return string(s[0]-('a'-'A')) + s[1:]
}
