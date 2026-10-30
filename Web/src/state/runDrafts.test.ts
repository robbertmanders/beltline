import { beforeEach, describe, expect, it } from "vitest";
import { RunDraftStore, runDraftsStorageKey } from "./runDrafts";

function memoryStorage() {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => { values.set(key, value); }
  };
}

describe("run draft store", () => {
  beforeEach(() => { window.sessionStorage.clear(); });

  it("isolates answers by run and source stage", () => {
    const store = new RunDraftStore(memoryStorage());
    store.setAnswer("run-a", "build", "first");
    store.setAnswer("run-a", "verify", "second");
    store.setAnswer("run-b", "build", "third");
    expect(store.stage("run-a", "build").answer).toBe("first");
    expect(store.stage("run-a", "verify").answer).toBe("second");
    expect(store.stage("run-b", "build").answer).toBe("third");
  });

  it("preserves whitespace exactly", () => {
    const store = new RunDraftStore(memoryStorage());
    store.setAnswer("run", "build", "  line one\n\nline two  ");
    expect(store.stage("run", "build").answer).toBe("  line one\n\nline two  ");
  });

  it("clears only the submitted answer and keeps edits made after submission", () => {
    const store = new RunDraftStore(memoryStorage());
    const version = store.setAnswer("run", "build", "submitted");
    store.setAnswer("run", "build", "edited later");
    store.clearAnswer("run", "build", version);
    expect(store.stage("run", "build").answer).toBe("edited later");

    const settled = store.setAnswer("run", "build", "again");
    store.clearAnswer("run", "build", settled);
    expect(store.stage("run", "build").answer).toBe("");
  });

  it("keeps decision notes per outcome and clears them with the stage's decision", () => {
    const store = new RunDraftStore(memoryStorage());
    store.setNote("run", "gate", "approve", "note for approve");
    store.setNote("run", "gate", "reject", "note for reject");
    store.setPendingOutcome("run", "gate", "approve");
    store.setSendMessage("run", "build", "message");
    expect(store.note("run", "gate", "approve")).toBe("note for approve");
    expect(store.note("run", "gate", "reject")).toBe("note for reject");
    store.clearDecide("run", "gate");
    expect(store.note("run", "gate", "approve")).toBe("");
    expect(store.stage("run", "gate").pendingOutcome).toBeNull();
    expect(store.stage("run", "build").sendMessage).toBe("message");
    store.clearRun("run");
    expect(store.stage("run", "build").sendMessage).toBe("");
  });

  it("round-trips through sessionStorage so a same-tab reload keeps drafts", () => {
    const storage = memoryStorage();
    const first = new RunDraftStore(storage);
    first.setSendMessage("run", "build", "keep me");
    const reloaded = new RunDraftStore(storage);
    expect(reloaded.stage("run", "build").sendMessage).toBe("keep me");
    expect(storage.getItem(runDraftsStorageKey)).toContain("keep me");
  });

  it("keeps a prompt stage's prompt and source choice across a reload until the decision clears them", () => {
    const storage = memoryStorage();
    const first = new RunDraftStore(storage);
    first.setPrompt("run", "prompt", "My unsent prompt");
    first.setSource("run", "prompt", "shared");
    const reloaded = new RunDraftStore(storage);
    expect(reloaded.stage("run", "prompt").prompt).toBe("My unsent prompt");
    expect(reloaded.stage("run", "prompt").source).toBe("shared");
    reloaded.clearDecide("run", "prompt");
    expect(reloaded.stage("run", "prompt").prompt).toBeNull();
    expect(reloaded.stage("run", "prompt").source).toBeNull();
  });
});
