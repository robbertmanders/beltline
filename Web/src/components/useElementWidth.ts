import { useEffect, useRef, useState, type RefObject } from "react";

/**
 * The laid-out width of an element in whole points, measured on mount and on every resize. It is 0 until the element
 * is mounted, so a caller that needs a fallback width applies it itself. It rounds down, so content sized to it never
 * overflows the element by a fraction of a point.
 */
export function useElementWidth<Element extends HTMLElement>(): [RefObject<Element | null>, number] {
  const ref = useRef<Element | null>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () => setWidth(Math.floor(element.getBoundingClientRect().width));
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  return [ref, width];
}
