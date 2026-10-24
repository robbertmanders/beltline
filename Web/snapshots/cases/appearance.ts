import type { SnapshotCase } from "../caseTypes";
import { agentsCases } from "./agents";
import { afkCases } from "./afk";
import { beltsCases } from "./belts";
import { galleryCases } from "./gallery";
import { libraryCases } from "./library";
import { overviewCases } from "./overview";
import { runCases } from "./run";
import { settingsCases } from "./settings";
import { workCases } from "./work";

// A bounded set of captures for the Dark appearance: one Dark representative per shared surface, reusing the page
// cases' fixtures and steps, plus the Light Settings and gallery captures. Each case is a copy of an existing one.
const every = [...overviewCases, ...workCases, ...runCases, ...beltsCases, ...agentsCases, ...libraryCases, ...afkCases, ...settingsCases, ...galleryCases];

function existing(id: string): SnapshotCase {
  const found = every.find(item => item.id === id);
  if (!found) throw new Error(`appearance cases reuse ${id}, which is not a page case`);
  return found;
}

const appearanceOf = (id: string, appearance: "light" | "dark", source: string): SnapshotCase => ({ ...existing(source), id, appearance });

export const appearanceCases: SnapshotCase[] = [
  appearanceOf("appearance-settings-light", "light", "settings-open"),
  {
    // Light stored, then the Dark option pressed from the keyboard: the focused option shows its outline in Dark.
    ...existing("settings-open"),
    id: "appearance-settings-keyboard-focus",
    appearance: "light",
    steps: [{ kind: "press", target: { role: "button", name: "Dark", exact: true }, key: "Enter" }],
    readyMarker: 'html[data-theme="dark"] [role="group"][aria-label="Appearance"]'
  },
  appearanceOf("appearance-settings-dark", "dark", "settings-open"),
  appearanceOf("appearance-settings-error-dark", "dark", "settings-inline-422"),
  appearanceOf("appearance-gallery-light", "light", "gallery"),
  appearanceOf("appearance-gallery-dark", "dark", "gallery"),
  appearanceOf("appearance-overview-dark", "dark", "overview-open"),
  // The Floor: the Floor button opens it, and it carries the tracks, forks and trigger captions.
  appearanceOf("appearance-overview-floor-boxes-dark", "dark", "overview-floor-open"),
  appearanceOf("appearance-work-dark", "dark", "work-open"),
  appearanceOf("appearance-belts-stage-form-dark", "dark", "belts-stage-form"),
  appearanceOf("appearance-agents-dark", "dark", "agents-open"),
  appearanceOf("appearance-library-share-dark", "dark", "library-share-belt"),
  appearanceOf("appearance-afk-dark", "dark", "afk-open"),
  appearanceOf("appearance-run-boxes-dark", "dark", "run-open"),
  appearanceOf("appearance-run-sessions-dark", "dark", "run-sessions-click")
];
