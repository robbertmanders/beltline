// The first message of a
// configuration conversation.

package core

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// AssistantPromptPrompt is the first message of a configuration conversation: what
// the assistant may change and how, the format, an index of the library files with
// what is wrong in them, how to check its own changes, and the user's request. problems is
// what LibraryCheck finds in the whole library; check is a
// shell command that prints the same for the files as they are then (nil when
// there is none).
func AssistantPromptPrompt(files map[string][]byte, request string, problems []string, check *string) string {
	contract := AssistantPromptTemplate()
	found := "None."
	if len(problems) > 0 {
		items := make([]string, 0, len(problems))
		for _, problem := range problems {
			items = append(items, "- "+problem)
		}
		found = strings.Join(items, "\n")
	}
	checking := "Beltline lists problems in the files you write beside this conversation. When the user passes one on, fix it with another version."
	if check != nil {
		checking = "After every change, run `" + *check + "` and fix what it reports with another version before you answer. It runs the checks the app's Save makes " +
			"over the newest version of every belt and agent, prints each problem after its file's path, or \"No problems.\", and exits non-zero when there are any."
	}
	library := "The library is empty: no belts and no agents."
	if len(files) > 0 {
		// Every file in full would cost the whole library before the first request; the files stay readable on disk.
		library = "The newest version of each definition, what it is, its flags and its earlier versions. " +
			"Read a file before you change it, copy from it or answer about its details.\n\n" + assistantIndex(files)
	}
	return contract + "\n\n" + AssistantReferenceText() +
		"\n\n## The library now\n" + library +
		"\n\n## Problems now\n" + found +
		"\n\n## Checking your changes\n" + checking +
		"\n\n## The user's request\n" + request
}

// assistantPromptLatest is the newest version of each definition plus every state
// file (current), and the paths of the rest (older).
func assistantPromptLatest(library map[string][]byte) (current []string, older []string) {
	newest := map[string]int{}
	for path := range library {
		n := assistantPromptVersion(path)
		if n == nil {
			continue
		}
		folder := assistantPromptFolder(path)
		newest[folder] = max(newest[folder], *n)
	}
	paths := slices.Sorted(maps.Keys(library))
	for _, path := range paths {
		n := assistantPromptVersion(path)
		if n != nil && *n != newest[assistantPromptFolder(path)] {
			older = append(older, path)
		} else {
			current = append(current, path)
		}
	}
	return current, older
}

// assistantPromptVersion is the number a `v<n>` file name carries, or nil.
func assistantPromptVersion(path string) *int {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	if !strings.HasPrefix(name, "v") {
		return nil
	}
	first := ""
	for _, piece := range strings.Split(name[1:], ".") {
		if piece != "" {
			first = piece
			break
		}
	}
	n, err := strconv.Atoi(first)
	if err != nil {
		return nil
	}
	return &n
}

// assistantPromptFolder is the first two components of a library path, which name a
// definition's folder.
func assistantPromptFolder(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, "/")
}
