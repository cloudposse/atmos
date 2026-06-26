import React, { useEffect, useRef, useState } from 'react';
import styles from './styles.module.css';

/**
 * A discrete yellow dot that marks an experimental feature in the docs sidebar,
 * with an instant, styled tooltip on hover that reads "experimental".
 *
 * The tooltip is `position: fixed` (coordinates computed from the dot's bounding
 * rect) so it escapes the sidebar's scroll/overflow clipping — a CSS pseudo-element
 * or an absolutely-positioned tooltip would be cut off. The dot's `aria-label` is
 * folded into the enclosing sidebar link's accessible name, so screen readers
 * announce "<command>, experimental feature" without needing the visual tooltip.
 *
 * Kept dependency-free (no framer-motion) and self-contained so it adds negligible
 * weight to the docs bundle, which renders the sidebar on every page.
 */
export default function ExperimentalDot(): JSX.Element {
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
    window.addEventListener('scroll', reposition, true);
    window.addEventListener('resize', reposition);
    return () => {
      window.removeEventListener('scroll', reposition, true);
      window.removeEventListener('resize', reposition);
    };
  }, [show]);

  return (
    <span
      ref={ref}
      className={styles.dot}
      role="img"
      aria-label="experimental feature"
      onMouseEnter={() => setShow(true)}
      onMouseLeave={() => setShow(false)}
    >
      {show && (
        <span
          role="tooltip"
          className={styles.tooltip}
          style={{ left: pos.x, top: pos.y }}
        >
          experimental
        </span>
      )}
    </span>
  );
}
