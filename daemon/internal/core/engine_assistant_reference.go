// What the
// configuration assistant needs to know about belts and agents beyond the files it
// can read, built from the model's own types.
//
// The literals are written here as raw strings, with § standing for each
// backtick (no reference text uses §), so the output stays byte for byte the same.

package core

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// AssistantReferenceBuiltInFolder is where the built-in agents' files are copied
// for the assistant, relative to the data folder.
const AssistantReferenceBuiltInFolder = "assistant/built-in"

// assistantReferenceQuote turns the § of a literal into a backtick.
func assistantReferenceQuote(text string) string {
	return strings.ReplaceAll(text, "§", "`")
}

// AssistantReferenceText is the whole reference: its sections in order.
func AssistantReferenceText() string {
	return strings.Join([]string{
		"## The format",
		assistantReferenceStages(),
		assistantReferenceWatches(),
		assistantReferenceActions(),
		assistantReferenceTriggers(),
		assistantReferencePlaceholders(),
		assistantReferenceBuiltIns(),
		assistantReferenceExample(),
	}, "\n\n")
}

// assistantReferenceInputLabels are the labels of a set of inputs, in raw-value order.
func assistantReferenceInputLabels(inputs map[Input]struct{}) []string {
	var raws []string
	for input := range inputs {
		raws = append(raws, string(input))
	}
	sort.Strings(raws)
	labels := make([]string, 0, len(raws))
	for _, raw := range raws {
		labels = append(labels, Input(raw).Label())
	}
	return labels
}

// assistantReferenceStages is the stage types, workspaces and fallbacks section.
func assistantReferenceStages() string {
	var workspaces, fallbacks []string
	for _, workspace := range AllWorkspace {
		workspaces = append(workspaces, "§"+string(workspace)+"§ ("+LowercasedFull(workspace.Label())+")")
	}
	for _, fallback := range AllFallback {
		fallbacks = append(fallbacks, "§"+string(fallback)+"§ ("+LowercasedFull(fallback.Label())+")")
	}
	fallbackList := strings.Join(fallbacks, ", ")
	var types []string
	for _, typ := range AllStageType {
		var what string
		switch typ {
		case StageTypeAgent:
			what = "runs the library agent it pins in §agent§. Its §runner§, §model§, §thinking§, §workspace§ and §slots§ start as the agent's, and it writes §<key>.md§; an empty §brief§ uses the agent's own. §alternates§, a list of {\"runner\", \"model\", \"thinking\"?} (an agent file's frontmatter lists them as §- runner:§ items under §alternates:§), are other settings its fresh sessions are spread across besides its own, to compare models: each one takes the entry fewest of the stage's running sessions use, a tie drawn at random, and a run coming back keeps the one it had; every model must be allowed. Its outcomes are the agent's, with §then§ set for this belt. §fallback§ says what happens when the session ends without a known outcome: " +
				fallbackList + "."
		case StageTypeApprove:
			what = "waits for the user to choose. Each outcome is a button, and its §when§ is the button's label."
		case StageTypePrompt:
			what = "waits for the user to write the run's task, usually as the first stage. Each outcome is a button, and its §when§ is the button's label; an outcome that moves the run on needs the prompt, which every later brief carries as the run's task, so none of its outcomes takes §withOutput§. §source§ (§shared§ or §withheld§, the default) is what the choice to give agents the work item's details starts as; withheld leaves only its reference in the briefs."
		case StageTypeWatch:
			what = "waits on something outside the app and needs no agent: never look for a \"watch\" agent. §watch§ names what it waits on (see Watches), and each outcome's §when§ names one of that target's events."
		}
		types = append(types, "- §"+string(typ)+"§ ("+typ.Title()+"): "+what)
	}
	var provided []string
	for _, kind := range AllTriggerKind {
		provided = append(provided, "§"+string(kind)+"§: "+strings.Join(assistantReferenceInputLabels(kind.Provides()), ", "))
	}
	worktree := WorkspaceWorktree
	produced := NewStage(StageArgs{Type: StageTypeAgent, Workspace: &worktree})
	producesText := strings.Join(assistantReferenceInputLabels(produced.Produces()), " and ")
	needsText := strings.Join(assistantReferenceInputLabels(WorkspaceWorktreeReadOnly.Needs()), "")
	text := assistantReferenceQuote(`### Stages
A stage's §type§ is one of:
` + strings.Join(types, "\n") + `

Workspaces: ` + strings.Join(workspaces, ", ") + `.

What a stage needs must come from the trigger or an earlier stage. Triggers provide ` + strings.Join(provided, "; ") + `. ` +
		`An agent in a §worktree§ produces ` + producesText + ` for the stages after it. ` +
		`A §worktreeReadOnly§ agent needs ` + needsText + `; a pull request watch needs ` + InputPullRequest.Label() +
		`, so on a belt that doesn't start from a review request it comes after an agent that opens one, such as Ship; a ticket watch needs ` + InputTicket.Label() + `.

Forks: a belt's §forks§ list where its stages split into routes, as {"id", "routes": [{"id", "name", "feeds"?}]}. Each stage on a route names it as its §route§; ` +
		`a fork's stages sit together in §stages§, route after route, and the stage after them is where the routes meet. A run takes the first route unless an ` +
		`outcome takes another with §route§; a fork at the very start of a belt starts every route. A route with §"feeds": true§ (never the first) feeds into the stage ` +
		`after the fork: nothing on the line splits into it, a run only gets there with a §goto§ to one of its stages or Send to, and it may never be reached; ` +
		`a fork for such branches has an empty first route, the line. A route whose last stage has no outcome that moves on past the fork ` +
		`(it goes back, holds or drops) ends there instead of meeting the others.`)
	return text
}

// assistantReferenceWatches is the watch targets and events section.
func assistantReferenceWatches() string {
	var targets []string
	for _, target := range AllWatchTarget {
		field := "\"watch\": \"" + string(target) + "\""
		if target == WatchTargetPullRequest {
			field = "leave §watch§ out (or \"" + string(target) + "\")"
		}
		var events []string
		for _, event := range target.Events() {
			events = append(events, "  - §"+event.When+"§: "+event.Label)
		}
		targets = append(targets, "- "+target.Label()+" (a new one is named \""+target.Title()+"\"): "+field+
			". Its events, the values its outcomes' §when§ may take:\n"+strings.Join(events, "\n"))
	}
	ticketTarget := WatchTargetTicket
	ticket := NewStage(StageArgs{ID: "watch-ticket", Type: StageTypeWatch, Outcomes: DefaultsTicketWatchOutcomes(), Watch: &ticketTarget})
	ticket.Outcomes = assistantReferenceReadable(ticket.Outcomes, ticket.ID)
	return assistantReferenceQuote(`### Watches
` + strings.Join(targets, "\n") + `

A pull request watch reports a merge, a close, a conflict or a branch behind its base, failing checks, and reviews; §` + string(WatchEventNone) + `§ is taken ` +
		`only when no other outcome applies. A §goto§ to §watch§ in an agent's outcomes finds the watch stage. A ticket watch on the run's issue or ticket:
` + "```json\n" + assistantReferenceJSON(ticket) + "\n```")
}

// assistantReferenceActions is the outcome actions section.
func assistantReferenceActions() string {
	var tones []string
	for _, tone := range AllTone {
		tones = append(tones, "§"+string(tone)+"§")
	}
	var list []string
	for _, action := range AllThenAction {
		var what string
		switch action {
		case ThenActionNext:
			what = "the next stage, or the end of the belt."
		case ThenActionGoto:
			what = "the stage named in §to§ (a stage id), or one per route in §to§ when it goes back into a fork."
		case ThenActionRoute:
			what = "the fork route(s) named in §route§."
		case ThenActionRetry:
			what = "this stage again."
		case ThenActionHold:
			what = "stop and wait for the user; §comment: true§ posts the output's questions on the issue."
		case ThenActionPark:
			what = "set the run aside until the user unparks it."
		case ThenActionDone:
			what = "finish the run here, as done: the work is complete but nothing was shipped (a PR review, say)."
		case ThenActionShip:
			what = "finish the run here, as shipped: the work has shipped. Use it only where the work has actually been delivered; Done is the default for a successful finish."
		case ThenActionDrop:
			what = "drop the run."
		}
		list = append(list, "- §"+string(action)+"§ ("+action.Label()+"): "+what)
	}
	return assistantReferenceQuote(`### What an outcome does
An outcome is {"id", "name", "when", "tone", "then"}; §tone§ is one of ` + strings.Join(tones, ", ") + `. ` +
		`§then§ is {"do": <action>} plus, where it applies, §to§, §route§, §withOutput: true§ (pass the output on as a note), §max§ (hold after that many uses on a run) and §comment§:
` + strings.Join(list, "\n"))
}

// assistantReferenceTriggers is the trigger kinds section.
func assistantReferenceTriggers() string {
	var list []string
	for _, kind := range AllTriggerKind {
		list = append(list, "- "+kind.Title()+": §"+assistantReferenceJSON(kind.Fresh())+"§")
	}
	return assistantReferenceQuote(`### Triggers
A belt's §triggers§ is an ordered list. Each entry is {"id", "trigger", "repos"} and keeps its own repositories, poll interval and kind settings. ` +
		`The same kind may appear more than once. §base§, §policies§, §afk§ and §stages§ stay on the belt and are shared. ` +
		`§repositoryBases§ is a map from each listed "owner/repository" to the branch that repository starts from instead of §base§; an empty or missing entry means §base§, and a missing branch means that repository's default. ` +
		`A key for a repository no entry lists is refused on save, and an empty value is dropped. A run takes these choices when it starts. ` +
		`A belt saved with one §trigger§ and §repos§ reads as a single entry whose id is "1". A new entry's id stays when it is edited or reordered. ` +
		`What a stage needs must be available from every entry, or from an earlier stage: an input one kind provides does not make another kind compatible. ` +
		`Each entry's §trigger§, as a new belt of that kind starts with it:
` + strings.Join(list, "\n"))
}

// assistantReferencePlaceholders is the placeholders section.
func assistantReferencePlaceholders() string {
	var fixed []string
	for _, entry := range PlaceholderHelpFixed() {
		fixed = append(fixed, "- §{"+entry.Name+"}§: "+entry.Description)
	}
	return assistantReferenceQuote(`### Briefs
A brief may use these fixed placeholders and §{<placeholder>}§ for each output-producing stage. That name is chosen on the stage and is the only name for its output; agent keys, agent ids and stage ids are not aliases. Use §{output}§ for the current stage's file.
` + strings.Join(fixed, "\n"))
}

// assistantReferenceBuiltIns is the built-in agents section.
func assistantReferenceBuiltIns() string {
	var list []string
	for _, agent := range DefaultsBuiltInAgents() {
		var outcomes []string
		for _, outcome := range agent.Outcomes {
			outcomes = append(outcomes, outcome.Name)
		}
		list = append(list, "- §"+agent.ID+"§ v"+strconv.Itoa(agent.Version)+", "+agent.Name+": §"+string(agent.Workspace)+"§; outcomes "+strings.Join(outcomes, ", "))
	}
	return assistantReferenceQuote(`### Built-in agents
The app ships these. One is in the library only when §agents/<id>/§ holds it; to use one that isn't, copy its file from ` +
		"§" + AssistantReferenceBuiltInFolder + `/<id>.md§ to §agents/<id>/v1.md§ first, then pin it.
` + strings.Join(list, "\n"))
}

// assistantReferenceExample is an example belt, encoded from the defaults a new
// belt starts with.
func assistantReferenceExample() string {
	belt := DefaultsBelt([]string{"owner/repo"}, "agent", DefaultsBuiltInAgents(), true)
	for index := range belt.Stages {
		belt.Stages[index].Outcomes = assistantReferenceReadable(belt.Stages[index].Outcomes, belt.Stages[index].ID)
	}
	return assistantReferenceQuote(`### A whole belt
The belt a new GitHub issues belt starts with: approval and watch stages, a fork after the watch and the routes back to it. ` +
		`It pins the built-in agents, so each must be in §agents/§ first; its runners and models are placeholders, so use ones the user allows.
` + "```json\n" + assistantReferenceJSON(belt) + "\n```")
}

// assistantReferenceReadable are outcomes with ids a person can read, unique
// within the stage.
func assistantReferenceReadable(outcomes []Outcome, stage string) []Outcome {
	out := make([]Outcome, 0, len(outcomes))
	for _, outcome := range outcomes {
		outcome := outcome.Clone()
		var slug strings.Builder
		for _, c := range Characters(LowercasedFull(outcome.Name)) {
			r, _ := utf8.DecodeRuneInString(c)
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				slug.WriteString(c)
			} else {
				slug.WriteRune('-')
			}
		}
		outcome.ID = stage + "-" + slug.String()
		out = append(out, outcome)
	}
	return out
}

// assistantReferenceJSON is value encoded as JSON, the reference's form: sorted
// keys, unescaped slashes and no spacing.
func assistantReferenceJSON(value any) string {
	data, err := jsonx.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
