import { useEffect, type RefObject } from "react";

/** Fits every recorded column inside the player while retaining its preferred font size when space allows. */
export default function useTerminalSize(
  ref: RefObject<HTMLPreElement | null>,
  columns: number,
) {
  useEffect(() => {
    const screen = ref.current;
    if (!screen || !Number.isFinite(columns) || columns <= 0) return;
    const context = document.createElement("canvas").getContext("2d");
    if (!context) return;
    let disposed = false;

    const fit = () => {
      if (disposed) return;
      // Read the stylesheet's preferred size again after responsive breakpoints change.
      screen.style.removeProperty("font-size");
      const style = getComputedStyle(screen);
      const preferred = parseFloat(style.fontSize);
      context.font = `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
      const cellWidth = context.measureText("0").width;
      const available =
        screen.clientWidth -
        parseFloat(style.paddingLeft) -
        parseFloat(style.paddingRight);
      if (available > 0 && cellWidth > 0) {
        // Reserve a pixel for subpixel rounding at the right edge.
        const size = Math.min(
          preferred,
          (preferred * Math.max(0, available - 1)) / (columns * cellWidth),
        );
        screen.style.fontSize = `${size}px`;
      }
    };

    fit();
    const observer =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(fit);
    observer?.observe(screen);
    window.addEventListener("resize", fit);
    void document.fonts?.ready.then(fit);
    return () => {
      disposed = true;
      observer?.disconnect();
      window.removeEventListener("resize", fit);
      screen.style.removeProperty("font-size");
    };
  }, [ref, columns]);
}
