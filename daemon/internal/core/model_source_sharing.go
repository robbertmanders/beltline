// SourceSharing: whether agents see the work item a run came from.

package core

// SourceSharing is whether a run's agents see the details of the work item it
// came from (its title, description, labels, parent and link), or only its
// reference. You choose it at a prompt stage.
type SourceSharing string

const (
	// SourceSharingShared gives agents the work item's details with your prompt.
	SourceSharingShared SourceSharing = "shared"
	// SourceSharingWithheld gives agents your prompt and the work item's reference only.
	SourceSharingWithheld SourceSharing = "withheld"
)

// AllSourceSharing lists every SourceSharing in declaration order (CaseIterable).
var AllSourceSharing = []SourceSharing{SourceSharingShared, SourceSharingWithheld}

// Known reports whether x is one of the cases of SourceSharing.
func (x SourceSharing) Known() bool { return enumKnown(x, AllSourceSharing) }

// Label is the name the editor shows for the choice.
func (x SourceSharing) Label() string {
	switch x {
	case SourceSharingShared:
		return "Shared with agents"
	case SourceSharingWithheld:
		return "Withheld from agents"
	}
	return ""
}
