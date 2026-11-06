import { useState } from "react";
import { useNavigate } from "react-router-dom";
import type { Belt, BeltContext, RunResponse } from "../../api/wire.gen";
import { useCommand } from "../../api/useCommand";
import { Icon } from "../../components/Icon";
import { Layer } from "../../components/Layer";
import { ErrorProblems } from "../../components/ProblemList";
import { manualEntries } from "./TriggerForm";

/**
 * Start a task on a saved, live manual trigger. It takes the saved belt's triggers and repositories, never the
 * unsent draft's. The daemon's task defaults (the first manual entry and its first repository) come from the
 * editor's context when the belt has no unsent changes; otherwise the saved belt is read here. Cancel sends
 * nothing; a rejected start keeps the sheet and its input; a started task closes the sheet and opens its run.
 * Opened from the top bar, it offers every belt the daemon says can start a task (`choices`).
 */
export function TaskSheet({ belt: initial, context, choices, onClose }: { belt: Belt; context?: BeltContext | null; choices?: readonly Belt[]; onClose: () => void }) {
  const navigate = useNavigate();
  const command = useCommand();
  const [beltId, setBeltId] = useState(initial.id);
  const belt = choices?.find(item => item.id === beltId) ?? initial;
  const ownContext = belt.id === initial.id ? context : null;
  const manual = ownContext?.taskEntries
    ? ownContext.taskEntries.map(item => ({ entry: { id: item.id, repos: item.repos }, position: item.position }))
    : manualEntries(belt);
  const [entryId, setEntryId] = useState(ownContext?.task?.entry?.id ?? manual[0]?.entry.id ?? "");
  const chosen = manual.find(item => item.entry.id === entryId) ?? manual[0];
  const entry = chosen?.entry;
  const repos = entry?.repos ?? [];
  const [repo, setRepo] = useState(ownContext?.task?.repo ?? repos[0] ?? "");
  const repository = repos.includes(repo) ? repo : repos[0] ?? "";
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");

  const chooseEntry = (next: string) => {
    setEntryId(next);
    const nextRepos = manual.find(item => item.entry.id === next)?.entry.repos ?? [];
    setRepo(nextRepos.includes(repository) ? repository : nextRepos[0] ?? "");
  };
  const chooseBelt = (next: string) => {
    setBeltId(next);
    const nextBelt = choices?.find(item => item.id === next);
    const first = nextBelt ? manualEntries(nextBelt)[0]?.entry : undefined;
    setEntryId(first?.id ?? "");
    setRepo(first?.repos?.[0] ?? "");
  };

  const titleReason = title.trim() === "" ? "Give the task a title." : null;
  const repoReason = repos.length === 0 ? "No repository is eligible for this task. Add one to the trigger first." : null;
  const reason = titleReason ?? repoReason;

  const start = () => {
    if (reason !== null || entry === undefined || command.pending) return;
    command.submit({
      path: "/api/v1/tasks",
      body: { belt: belt.id, entry: entry.id, repo: repository, title, body },
      affects: ["runs", "state"],
      publish: (_queryClient, result: RunResponse) => {
        onClose();
        navigate(`/runs/${encodeURIComponent(result.run.id)}`);
      }
    });
  };

  return <Layer label="Start a task" inverse onClose={onClose}>
    <label className="h2" htmlFor="task-title">What should it do?</label>
    <input className="nt-q" id="task-title" aria-label="Title" data-autofocus value={title} placeholder="Retry failed exports" autoComplete="off"
      onChange={event => setTitle(event.target.value)} onKeyDown={event => { if (event.key === "Enter") start(); }} />
    <div className="cols2" style={{ marginTop: 36 }}>
      {choices && choices.length > 0 && <div className="fld"><label htmlFor="task-belt">Belt</label>
        <select className="sel" id="task-belt" value={belt.id} onChange={event => chooseBelt(event.target.value)}>
          {choices.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
        </select>
        <span className="help">Runs {belt.stages.map(stage => stage.name).join(", ")}.</span>
      </div>}
      {manual.length > 1 && <div className="fld"><label htmlFor="task-trigger">Trigger</label>
        <select className="sel" id="task-trigger" aria-label="Trigger" value={entry?.id ?? ""} onChange={event => chooseEntry(event.target.value)}>
          {manual.map(item => <option key={item.entry.id} value={item.entry.id}>Manual task trigger {item.position}</option>)}
        </select>
      </div>}
      <div className="fld"><label htmlFor="task-repo">Repository</label>
        <select className="sel mono" id="task-repo" aria-label="Repository" value={repository} disabled={repos.length === 0} onChange={event => setRepo(event.target.value)}>
          {repos.map(item => <option key={item} value={item}>{item}</option>)}
        </select>
        {repos.length === 0 && <span className="help">{belt.name} names no repositories yet.</span>}
      </div>
    </div>
    <div className="fld" style={{ marginTop: 28 }}><label htmlFor="task-body">Details</label>
      <textarea className="ta" id="task-body" aria-label="What to do" value={body} placeholder="Anything the agent should know: where to look, what not to touch."
        onChange={event => setBody(event.target.value)} />
    </div>
    <div className="acts" style={{ marginTop: 28 }}>
      <button type="button" className="btn fill" disabled={reason !== null || command.pending} aria-describedby={reason ? "task-start-reason" : undefined} onClick={start}><Icon name="play" />Start</button>
      <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
      <span className="why" id="task-start-reason">{reason ?? "Enter starts it too."}</span>
    </div>
    {command.error !== null && command.error !== undefined && <ErrorProblems error={command.error} />}
  </Layer>;
}
