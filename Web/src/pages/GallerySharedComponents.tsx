import { SheetPanel } from "../components/Sheet";
import { MarkdownBlocks } from "../components/MarkdownBlocks";
import { ModelPicker } from "../components/ModelPicker";
import { LibraryColumn, type LibraryRow } from "../components/LibraryColumn";
import { ProblemList } from "../components/ProblemList";
import type { MarkdownBlock, ModelChoice } from "../api/wire.gen";

// Sample content for the shared building blocks. It is illustrative data, not a daemon response.

const longOutput: MarkdownBlock[] = [
  { type: "heading", level: 1, text: "Release notes" },
  { type: "paragraph", text: "A paragraph with enough words to show that short lines reflow around the output column. This sentence is deliberately kept on its own line because the next word would not fit.\nhard line two" },
  { type: "item", marker: "•", text: "first item with a **strong** word and a `code span`" },
  { type: "item", marker: "1.", text: "numbered item with a [safe link](https://example.com/notes)" },
  { type: "code", text: "let greeting = \"hello  \"\nprint(greeting)" }
];

const unsafeOutput: MarkdownBlock[] = [
  { type: "paragraph", text: "Links that stay text: [script](javascript:alert(1)) and [data](data:text/html,x). Raw <b>markup</b> stays visible." }
];

const choices: ModelChoice[] = [
  { id: "opus-5.5-1m", name: "Opus 5.5", contextTokens: 1000000 },
  { id: "opus-5.5-250k", name: "Opus 5.5", contextTokens: 250000 },
  { id: "fixture-model", name: "Fixture model (demo)" }
];

const longRows: LibraryRow[] = Array.from({ length: 14 }, (_, index) => ({
  id: `agent-${index}`,
  label: index === 3 ? "Resolve conflicts in a very long agent name that must wrap inside its row" : `Agent ${index + 1}`,
  detail: `Agent · v${(index % 3) + 1}`
}));

function Caption({ children }: { children: string }) {
  return <p className="gallery-sample-caption paper-caps">{children}</p>;
}

/** Shared page building blocks in their states. Each block renders the same component the pages use. */
export function GallerySharedComponents() {
  return <section className="gallery-section" id="components" aria-labelledby="components-heading">
    <header className="gallery-section__heading"><span className="paper-tag">05</span><div><h2 id="components-heading">Shared components</h2><p>Markdown blocks, problems, model choices, library columns and a sheet, in the states the pages show.</p></div></header>
    <div className="gallery-component-grid">
      <article className="paper-card gallery-component-card" data-column="gallery-markdown">
        <h3>Markdown blocks</h3>
        <Caption>Long output · headings, items, code</Caption>
        <MarkdownBlocks blocks={longOutput} />
        <Caption>Unsafe links stay text</Caption>
        <MarkdownBlocks blocks={unsafeOutput} />
      </article>
      <article className="paper-card gallery-component-card" data-column="gallery-problems">
        <h3>Problems</h3>
        <Caption>Daemon rejection, in order</Caption>
        <ProblemList message={"belt a has no stages\nagent b is missing"} problems={["belt a has no stages", "agent b is missing"]} className="control-error" />
      </article>
      <article className="paper-card gallery-component-card" data-column="gallery-models">
        <h3>Model choices</h3>
        <ModelPicker id="gallery-model-ready" label="Ready" runnerLabel="Pi" choices={choices} allowedModels={choices.map(choice => choice.id)} value="opus-5.5-250k" />
        <ModelPicker id="gallery-model-default" label="Runner default" runnerLabel="Pi" choices={choices} allowedModels={[]} value="" />
        <ModelPicker id="gallery-model-empty" label="No models" runnerLabel="Copilot" choices={[]} allowedModels={[]} value="" />
        <ModelPicker id="gallery-model-frozen" label="Frozen" runnerLabel="Pi" choices={choices} value="fixture-model" state="frozen" />
        <ModelPicker id="gallery-model-readonly" label="Publisher" runnerLabel="Pi" choices={[]} value="Vendor/Model_ID.v2:latest" state="readonly" />
      </article>
      <article className="paper-card gallery-component-card gallery-library-sample" data-column="gallery-library">
        <h3>Library column</h3>
        <Caption>Independently scrolling rows with Add</Caption>
        <div className="gallery-library-frame">
          <LibraryColumn title="Agents" foldKey="gallery-agents" rows={longRows} selectedId="agent-3" onSelect={() => {}} onAdd={() => {}} addLabel="Add agent"
            empty={<p>No agents yet.</p>} footer={<span>14 agents</span>} />
        </div>
      </article>
    </div>
    <div className="gallery-sheet-sample" data-column="gallery-sheet">
      <Caption>Sheet · New · white titled panel on the dark screen surface</Caption>
      <SheetPanel title="Edit model" titleId="gallery-sheet-title"
        footer={<><button type="button" className="btn">Cancel</button><button type="button" className="btn fill">Save</button></>}>
        <ModelPicker id="gallery-sheet-model" label="Model" runnerLabel="Pi" choices={choices} allowedModels={choices.map(choice => choice.id)} value="opus-5.5-1m" />
        <ProblemList message="The daemon rejected this request." problems={["model opus-5.5-1m is not allowed for copilot"]} />
        <pre className="markdown-code"><code>{"print(\"literal **code** stays literal\")"}</code></pre>
      </SheetPanel>
    </div>
  </section>;
}
