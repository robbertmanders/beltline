/** Where the last click landed, so a sheet or the run page can open as a circle from it. */
let lastClick: { x: number; y: number } | null = null;

export const rememberClick = {
  /** Listens for clicks on the whole page; returns the function that stops listening. */
  listen(): () => void {
    const onDown = (event: PointerEvent) => { lastClick = { x: event.clientX, y: event.clientY }; };
    window.addEventListener("pointerdown", onDown, true);
    return () => window.removeEventListener("pointerdown", onDown, true);
  },
  /** The last click, or the middle of the window when the keyboard opened it. */
  last(): { x: number; y: number } {
    return lastClick ?? { x: (typeof window === "undefined" ? 0 : window.innerWidth) / 2, y: (typeof window === "undefined" ? 0 : window.innerHeight) / 2 };
  }
};
