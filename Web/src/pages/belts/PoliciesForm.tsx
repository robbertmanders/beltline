import type { Policies } from "../../api/wire.gen";
import { useDraftEditor } from "./draftEditor";

/**
 * The belt's policies, as the native form shows them: only those the belt's triggers can use, worded for them,
 * with the section's heading and note from the daemon's context. A box shows the draft's value, and a change
 * sends the whole policies object as one policies-update.
 */
export function PoliciesForm() {
  const editor = useDraftEditor();
  const section = editor.context?.policySection ?? null;
  if (!section) return null;
  const current = editor.belt.policies;
  return <section className="belt-policies" aria-label="Policies">
    <h3>{section.heading}</h3>
    <ul className="policy-list">
      {section.options.map(option => {
        const key = option.key as keyof Policies;
        return <li key={option.key}>
          <label className="tg">
            <input type="checkbox" checked={current[key] === true} disabled={editor.blocked !== null} aria-describedby={`policy-${option.key}`}
              onChange={event => editor.operate("policies-update", { policies: { ...current, [key]: event.target.checked } })} />
            <span className="t" aria-hidden="true" /><span>{option.label}</span>
          </label>
          <p id={`policy-${option.key}`} className="policy-detail">{option.detail}</p>
        </li>;
      })}
    </ul>
    {section.note && <p className="field-note">{section.note}</p>}
  </section>;
}
