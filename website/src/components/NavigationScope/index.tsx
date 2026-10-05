import React from "react";
import styles from "./styles.module.css";

/** Explain a YAML scope without making the annotation look like another key. */
export default function NavigationScope({
  scope,
}: {
  scope?: unknown;
}): JSX.Element | null {
  return typeof scope === "string" ? (
    <span className={styles.scope}>{scope}</span>
  ) : null;
}
