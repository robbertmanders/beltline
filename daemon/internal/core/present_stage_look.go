// The stage look a box draws with: its icon and tint, and the trigger glyph.

package core

// StageLook is what a stage box shows besides its words: its icon and colour.
type StageLook struct {
	Icon StageIcon `json:"icon"`
	Tint Tint      `json:"tint"`
}

// StageLookOf is the look of a stage box. An agent stage's look comes from its agent; approvals and watches
// have their own.
func StageLookOf(stage Stage, agent *Agent) StageLook {
	switch stage.Kind() {
	case StageKindAgent:
		look := StageLook{Icon: StageIconChip, Tint: TintGrey}
		if agent != nil {
			look.Icon = agent.Glyph()
			look.Tint = agent.Tint
		}
		return look
	case StageKindGate:
		if stage.Type == StageTypePrompt {
			return StageLook{Icon: StageIconPen, Tint: TintGold}
		}
		return StageLook{Icon: StageIconApprove, Tint: TintGold}
	default:
		icon := StageIconMerge
		if stage.WatchesTicket() {
			icon = StageIconEye
		}
		return StageLook{Icon: icon, Tint: TintTeal}
	}
}

// StageLookTrigger is a trigger's glyph; nil is the issues trigger.
func StageLookTrigger(kind *TriggerKind) StageIcon {
	if kind == nil {
		return StageIconBolt
	}
	switch *kind {
	case TriggerKindJira:
		return StageIconJira
	case TriggerKindSchedule:
		return StageIconClock
	case TriggerKindManual:
		return StageIconHand
	case TriggerKindReviews:
		return StageIconEye
	default:
		return StageIconBolt
	}
}
