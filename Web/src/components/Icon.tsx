/** The line icons the pages draw at 17px, on a 24 × 24 grid. */
const paths = {
  overview: "M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z",
  work: "M4 6h16M4 12h16M4 18h10",
  insights: "M5 19V11M11 19V5M17 19v-6M3 19h18",
  belts: "M3 12h4M17 12h4M7 9h10v6H7z",
  agents: "M12 4.5a3.5 3.5 0 1 1 0 7a3.5 3.5 0 1 1 0-7M5 20c1.5-3.5 4-5 7-5s5.5 1.5 7 5",
  library: "M5 4v16M10 4v16M15 5l4 15",
  afk: "M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z",
  settings: "M4 7h10M18 7h2M4 17h4M12 17h8M16 5a2 2 0 1 1 0 4a2 2 0 1 1 0-4M10 15a2 2 0 1 1 0 4a2 2 0 1 1 0-4",
  search: "M11 4.5a6.5 6.5 0 1 1 0 13a6.5 6.5 0 1 1 0-13M16 16l4.5 4.5",
  plus: "M12 5v14M5 12h14",
  minus: "M5 12h14",
  check: "M5 12.5l4.5 4.5L19 7",
  back: "M19 12H5M11 6l-6 6 6 6",
  pause: "M8 5v14M16 5v14",
  play: "M7 5l12 7-12 7z",
  stop: "M6 6h12v12H6z",
  send: "M4 12l16-8-6 16-3-7z",
  undo: "M9 14L4 9l5-5M4 9h11a5 5 0 0 1 0 10h-3",
  x: "M6 6l12 12M18 6L6 18",
  retry: "M20 11a8 8 0 0 0-14.6-4.5M4 4v3h3M4 13a8 8 0 0 0 14.6 4.5M20 20v-3h-3",
  open: "M5 12h14M13 6l6 6-6 6",
  down: "M12 4v12M6 10l6 6 6-6M5 20h14",
  up: "M12 19V5M6 11l6-6 6 6",
  dn: "M12 5v14M6 13l6 6 6-6",
  ext: "M14 4h6v6M20 4l-9 9M18 14v6H4V6h6",
  menu: "M4 7h16M4 12h16M4 17h16",
  term: "M4 5h16v14H4zM7 9l3 3-3 3M12 15h5",
  folder: "M3 6h6l2 2h10v11H3z",
  code: "M9 7l-5 5 5 5M15 7l5 5-5 5",
  branch: "M6 3v12M6 15a3 3 0 1 0 0 6a3 3 0 1 0 0-6M18 3a3 3 0 1 0 0 6a3 3 0 1 0 0-6M18 9c0 5-6 4-11 7",
  pr: "M6 3a3 3 0 1 1 0 6a3 3 0 1 1 0-6M6 9v12M18 21a3 3 0 1 1 0-6a3 3 0 1 1 0 6M18 15V8a2 2 0 0 0-2-2h-5M13 3l-2 3 2 3",
  issue: "M12 3a9 9 0 1 1 0 18a9 9 0 1 1 0-18M12 8v5M12 16v.5",
  chat: "M4 5h16v11H9l-5 4z",
  trash: "M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13",
  sync: "M20 11a8 8 0 0 0-14.6-4.5M4 4v3h3M4 13a8 8 0 0 0 14.6 4.5M20 20v-3h-3",
  copy: "M8 8h11v11H8zM5 16V5h11",
  more: "M5 12h.01M12 12h.01M19 12h.01",
  edit: "M4 20h4L19 9l-4-4L4 16zM13.5 6.5l4 4"
} as const;

export type IconName = keyof typeof paths;

export function Icon({ name, className = "" }: { name: IconName; className?: string }) {
  return <svg className={`ic ${className}`} viewBox="0 0 24 24" aria-hidden="true"><path d={paths[name]} /></svg>;
}
