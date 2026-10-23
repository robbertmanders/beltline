package core

import (
	"slices"
	"strings"
)

// TaskDraftEntry is a manual entry on a belt, with the position shown on the form.
type TaskDraftEntry struct {
	ID       string   `json:"id"`
	Position int      `json:"position"`
	Repos    []string `json:"repos"`
}

// TaskDraftSelection is what the selected entry's form shows and whether Start can be pressed.
type TaskDraftSelection struct {
	Entry    *TaskDraftEntry `json:"entry,omitzero"`
	Repo     string          `json:"repo"`
	CanStart bool            `json:"canStart"`
	Problem  *string         `json:"problem,omitzero"`
}

// TaskDraftManuals are the belt's manual entries in order, with their original positions.
func TaskDraftManuals(belt Belt) []TaskDraftEntry {
	entries := []TaskDraftEntry{}
	for index, entry := range belt.Triggers {
		if entry.Trigger.Kind == TriggerKindManual {
			repos := cloneStrings(entry.Repos)
			if repos == nil {
				repos = []string{}
			}
			entries = append(entries, TaskDraftEntry{ID: entry.ID, Position: index, Repos: repos})
		}
	}
	return entries
}

// TaskDraftEntryID is the entry the form should select: the retained one when it exists,
// otherwise the first manual entry.
func TaskDraftEntryID(selected *string, belt Belt) *string {
	manuals := TaskDraftManuals(belt)
	if selected != nil {
		for _, manual := range manuals {
			if manual.ID == *selected {
				return new(*selected)
			}
		}
	}
	if len(manuals) == 0 {
		return nil
	}
	return new(manuals[0].ID)
}

// TaskDraftRepo keeps a repository the selected entry still has; otherwise its first.
func TaskDraftRepo(repo string, entryID *string, belt Belt) string {
	entry := taskDraftManual(entryID, belt)
	if entry == nil {
		return ""
	}
	for _, r := range entry.Repos {
		if r == repo {
			return repo
		}
	}
	if len(entry.Repos) == 0 {
		return ""
	}
	return entry.Repos[0]
}

// TaskDraftCanStart reports whether a task can start: a title must not be blank, and the
// repository must be one of the entry's.
func TaskDraftCanStart(title, repo string, repos []string) bool {
	if strings.TrimSpace(title) == "" || repo == "" {
		return false
	}
	return slices.Contains(repos, repo)
}

// TaskDraftSelectionOf is the whole form decision for one belt and its current input.
func TaskDraftSelectionOf(belt Belt, entryID *string, repo, title string) TaskDraftSelection {
	resolved := TaskDraftEntryID(entryID, belt)
	entry := taskDraftManual(resolved, belt)
	var repos []string
	if entry != nil {
		repos = entry.Repos
	}
	chosenRepo := TaskDraftRepo(repo, resolved, belt)
	var problem *string
	switch {
	case strings.TrimSpace(title) == "":
		problem = new("Give the task a title.")
	case chosenRepo == "":
		problem = new("This trigger has no repository to start the task in.")
	}
	return TaskDraftSelection{
		Entry: entry, Repo: chosenRepo, CanStart: TaskDraftCanStart(title, chosenRepo, repos), Problem: problem,
	}
}

// taskDraftManual is the manual entry with the given id, or nil when there is none; a nil id
// matches nothing. Shared by TaskDraftRepo and TaskDraftSelectionOf.
func taskDraftManual(id *string, belt Belt) *TaskDraftEntry {
	if id == nil {
		return nil
	}
	for _, manual := range TaskDraftManuals(belt) {
		if manual.ID == *id {
			found := manual
			return &found
		}
	}
	return nil
}
