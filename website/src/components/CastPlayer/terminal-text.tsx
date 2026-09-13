import React from "react";
import styles from "./styles.module.css";

// Draw terminal rails within their cells so font fallback and line spacing
// cannot shift their joints or extend them into neighboring rows.
const BOX_PATHS: Record<string, string> = {
  "─": "M0 1H2",
  "│": "M1 0V2",
  "┌": "M1 2V1H2",
  "┐": "M0 1H1V2",
  "└": "M1 0V1H2",
  "┘": "M0 1H1V0",
  "├": "M1 0V2M1 1H2",
  "┤": "M1 0V2M0 1H1",
  "┬": "M0 1H2M1 1V2",
  "┴": "M0 1H2M1 0V1",
  "┼": "M0 1H2M1 0V2",
  "╭": "M1 2V1.5Q1 1 1.5 1H2",
  "╮": "M0 1H0.5Q1 1 1 1.5V2",
  "╰": "M1 0V0.5Q1 1 1.5 1H2",
  "╯": "M0 1H0.5Q1 1 1 0.5V0",
};

export default function renderTerminalText(text: string) {
  return text
    .split(/([─│┌┐└┘├┤┬┴┼╭╮╰╯█●○\u2800-\u28ff])/u)
    .map((part, index) => {
      const path = Object.hasOwn(BOX_PATHS, part) ? BOX_PATHS[part] : undefined;
      const graphic = path || part === "█";
      if (!graphic && !/^[●○\u2800-\u28ff]$/u.test(part)) return part;
      return (
        <span key={index} className={styles.terminalCell}>
          <span className={graphic ? styles.drawnGlyph : undefined}>
            {part}
          </span>
          {graphic && (
            <svg
              viewBox="0 0 2 2"
              preserveAspectRatio="none"
              aria-hidden="true"
            >
              {path ? (
                <path
                  d={path}
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1"
                  vectorEffect="non-scaling-stroke"
                />
              ) : (
                <rect width="2" height="2" fill="currentColor" />
              )}
            </svg>
          )}
        </span>
      );
    });
}
