import { FitAddon } from "@xterm/addon-fit";
import { Terminal as XTerm, type IDisposable } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { useEffect, useRef, useState } from "react";
import { TerminalSocket, type TerminalStatus, type TerminalTarget, terminalSocketUrl } from "../api/terminalSocket";

/** The part of an xterm terminal this component uses, so tests can stand in a double. */
export interface TerminalLike {
  readonly cols: number;
  readonly rows: number;
  open(parent: HTMLElement): void;
  loadAddon(addon: unknown): void;
  write(data: Uint8Array): void;
  onData(listener: (data: string) => void): IDisposable;
  dispose(): void;
}

export interface FitLike {
  fit(): void;
  dispose(): void;
}

export interface TerminalProps {
  target: TerminalTarget;
  /** The daemon is offline: no socket opens, input stays off and Retry is disabled. */
  offline?: boolean;
  label?: string;
  createTerminal?: () => TerminalLike;
  createFit?: () => FitLike;
  socketFactory?: (url: string) => WebSocket;
}

/** The screen's colours, from the theme tokens (tokens.css), which keep them the same in both appearances. */
function screenColour(token: string, fallback: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(token).trim() || fallback;
}

function defaultTerminal(): TerminalLike {
  const foreground = screenColour("--screen-text", "#EFE2DC");
  return new XTerm({
    convertEol: false,
    cursorBlink: false,
    fontFamily: "\"Geist Mono\", monospace",
    fontSize: 12,
    scrollback: 5000,
    theme: { background: screenColour("--screen", "#22070A"), foreground, cursor: foreground, selectionBackground: screenColour("--screen-dim", "#B89A9C") }
  });
}

const statusText: Record<TerminalStatus, string> = {
  connecting: "Connecting to the session…",
  live: "Connected",
  offline: "Terminal offline. Input is off."
};

/**
 * One attachment to a daemon terminal, drawn as the dark screen. Each attachment owns its terminal, fit
 * add-on, resize observer, input listener and socket, and disposes all of them when its target changes,
 * the daemon goes offline, or the component unmounts. Retry starts a new attachment with input off.
 */
export function Terminal({ target, offline = false, label = "Terminal", createTerminal = defaultTerminal, createFit = () => new FitAddon(), socketFactory }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const factories = useRef({ createTerminal, createFit, socketFactory });
  factories.current = { createTerminal, createFit, socketFactory };
  const [status, setStatus] = useState<TerminalStatus>("connecting");
  const [writable, setWritable] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const session = target.session;
  const takeOver = target.takeOver;

  useEffect(() => {
    const container = containerRef.current;
    if (!container || offline) {
      setStatus(offline ? "offline" : "connecting");
      setWritable(false);
      return undefined;
    }
    const current = factories.current;
    const terminal = current.createTerminal();
    const fit = current.createFit();
    terminal.open(container);
    terminal.loadAddon(fit);
    fit.fit();
    setStatus("connecting");
    setWritable(false);

    const socket: TerminalSocket = new TerminalSocket({
      url: terminalSocketUrl(window.location, { session, takeOver }),
      socketFactory: current.socketFactory,
      onOutput: bytes => terminal.write(bytes),
      onOpen: () => socket.sendResize(terminal.cols, terminal.rows),
      onStatus: setStatus,
      onWritable: setWritable
    });
    const input = terminal.onData(data => socket.sendInput(data));
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => {
      fit.fit();
      socket.sendResize(terminal.cols, terminal.rows);
    });
    observer?.observe(container);
    return () => {
      observer?.disconnect();
      input.dispose();
      socket.close();
      fit.dispose();
      terminal.dispose();
    };
  }, [session, takeOver, offline, attempt]);

  const inputState = writable && status === "live" ? "on" : "off";
  return <div className="terminal" data-status={status} data-input={inputState}>
    <div className="terminal-status" role="status">
      {status === "live" ? (writable ? "Connected · input on" : "Watching · read only") : statusText[status]}
    </div>
    <div ref={containerRef} className="terminal-screen" role="region" aria-label={label} />
    {status === "offline" && <button type="button" className="link terminal-retry" disabled={offline} onClick={() => setAttempt(value => value + 1)}>Retry</button>}
  </div>;
}
