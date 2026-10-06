import React, { useEffect, useRef, useState } from "react";
import styles from "./styles.module.css";

/** Small lifecycle marker with an accessible label and an unclipped tooltip. */
export type FeatureStatus = "experimental" | "deprecated";

export default function FeatureStatusDot({
  status,
}: {
  status: FeatureStatus;
}): JSX.Element {
  const ref = useRef<HTMLSpanElement>(null);
  const [show, setShow] = useState(false);
  const [pos, setPos] = useState<{ x: number; y: number }>({ x: 0, y: 0 });

  const place = () => {
    const el = ref.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    setPos({ x: rect.left + rect.width / 2, y: rect.top });
  };

  // While visible, keep the fixed-positioned tooltip glued to the dot as the
  // sidebar scrolls or the window resizes.
  useEffect(() => {
    if (!show) return undefined;
    place();
    const reposition = () => place();
    window.addEventListener("scroll", reposition, true);
    window.addEventListener("resize", reposition);
    return () => {
      window.removeEventListener("scroll", reposition, true);
      window.removeEventListener("resize", reposition);
    };
  }, [show]);

  return (
    <span
      ref={ref}
      className={`${styles.dot} ${styles[status]}`}
      role="img"
      aria-label={`${status} feature`}
      onMouseEnter={() => setShow(true)}
      onMouseLeave={() => setShow(false)}
      onFocus={() => setShow(true)}
      onBlur={() => setShow(false)}
      tabIndex={0}
    >
      {show && (
        <span
          role="tooltip"
          className={styles.tooltip}
          style={{ left: pos.x, top: pos.y }}
        >
          {status}
        </span>
      )}
    </span>
  );
}
