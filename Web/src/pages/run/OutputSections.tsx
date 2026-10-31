import { useId, useState, type ReactNode } from "react";
import type { AttemptOutputSection, MarkdownBlock } from "../../api/wire.gen";
import { MarkdownBlocks } from "../../components/MarkdownBlocks";
import { readOutputFolds, writeOutputFolds, type OutputFolds } from "../../state/outputFolds";

const headingTags = ["h1", "h2", "h3", "h4", "h5", "h6"] as const;

/** The section's heading: a real heading element at its level, or a plain box for the Output preamble (level 0). */
function SectionHead({ level, children }: { level: number; children: ReactNode }) {
  if (level < 1) return <div className="output-section-head">{children}</div>;
  const Heading = headingTags[Math.min(level, 6) - 1]!;
  return <Heading className="output-section-head">{children}</Heading>;
}

/**
 * An agent attempt's output as a title and folding sections. The title stays open; each section's heading is a
 * disclosure button that opens only its own body. A choice is kept per run and attempt in this browser, and a
 * section with no saved choice starts closed. The parent keys this by run and attempt, so each one reads its own.
 */
export function OutputSections({ runId, attemptId, title, sections }: {
  runId: string;
  attemptId: string;
  title?: MarkdownBlock;
  sections: readonly AttemptOutputSection[];
}) {
  const baseId = useId();
  const [folds, setFolds] = useState<OutputFolds>(() => readOutputFolds(runId, attemptId));
  const toggle = (sectionId: string) => {
    const next = { ...folds, [sectionId]: !(folds[sectionId] === true) };
    setFolds(next);
    writeOutputFolds(runId, attemptId, next);
  };
  return <div className="run-output-sections">
    {title && <MarkdownBlocks blocks={[title]} />}
    {sections.map((section, index) => {
      const open = folds[section.id] === true;
      const bodyId = `${baseId}-section-${index}`;
      return <div key={section.id} className={`output-section${open ? " is-open" : ""}`}>
        <SectionHead level={section.level}>
          <button type="button" className="output-section-toggle" aria-expanded={open} aria-controls={bodyId}
            onClick={() => toggle(section.id)}>
            <span className="output-section-chevron" aria-hidden="true">›</span>
            <span className="output-section-label">{section.label}</span>
          </button>
        </SectionHead>
        <div id={bodyId} className="output-section-body" hidden={!open}>
          <MarkdownBlocks blocks={section.blocks} />
        </div>
      </div>;
    })}
  </div>;
}
