import React, { useEffect, useRef, useState } from "react";
import { RiCheckLine, RiFileCopyLine } from "react-icons/ri";
import styles from "./example-source.module.css";

export default function ExampleCopyButton({ text }: { text: string }) {
  const [status, setStatus] = useState("");
  const timer = useRef<ReturnType<typeof setTimeout>>();
  useEffect(() => () => clearTimeout(timer.current), []);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setStatus("Copied");
    } catch {
      setStatus("Copy unavailable");
    }
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setStatus(""), 2000);
  }
  return (
    <button
      className={styles.copy}
      type="button"
      onClick={copy}
      aria-label={status || "Copy file contents"}
      title={status || "Copy file contents"}
    >
      {status === "Copied" ? <RiCheckLine /> : <RiFileCopyLine />}
      <span className={styles.srOnly} role="status">
        {status}
      </span>
    </button>
  );
}
