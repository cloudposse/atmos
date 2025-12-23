import React, { useState, useRef, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import styles from './styles.module.css';

interface TooltipProps {
  content: string;
  children: React.ReactNode;
}

export default function Tooltip({ content, children }: TooltipProps): JSX.Element {
  const [isVisible, setIsVisible] = useState(false);
  const [position, setPosition] = useState({ x: 0, y: 0 });
  const [isDark, setIsDark] = useState(false);
  const triggerRef = useRef<HTMLSpanElement>(null);
  const tooltipRef = useRef<HTMLDivElement>(null);

  // Check theme from DOM attribute directly for reliable dark mode detection.
  useEffect(() => {
    const checkTheme = () => {
      const theme = document.documentElement.getAttribute('data-theme');
      setIsDark(theme === 'dark');
    };

    checkTheme();

    // Watch for theme changes.
    const observer = new MutationObserver(checkTheme);
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    });

    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (isVisible && triggerRef.current && tooltipRef.current) {
      const triggerRect = triggerRef.current.getBoundingClientRect();
      const tooltipRect = tooltipRef.current.getBoundingClientRect();

      // Position above the trigger, centered.
      let x = triggerRect.left + triggerRect.width / 2 - tooltipRect.width / 2;
      const y = triggerRect.top - tooltipRect.height - 8;

      // Keep tooltip within viewport horizontally.
      if (x < 8) x = 8;
      if (x + tooltipRect.width > window.innerWidth - 8) {
        x = window.innerWidth - tooltipRect.width - 8;
      }

      setPosition({ x, y });
    }
  }, [isVisible]);

  // Theme-aware colors - keep dark tooltip in both modes for consistency.
  // In dark mode, use a slightly lighter dark to contrast with page background.
  const tooltipBg = isDark ? '#2a2a3e' : '#1e1e2e';
  const tooltipColor = '#f0f0f0';

  return (
    <span
      ref={triggerRef}
      className={styles.tooltipTrigger}
      onMouseEnter={() => setIsVisible(true)}
      onMouseLeave={() => setIsVisible(false)}
      onFocus={() => setIsVisible(true)}
      onBlur={() => setIsVisible(false)}
    >
      {children}
      <AnimatePresence>
        {isVisible && (
          <motion.div
            ref={tooltipRef}
            className={styles.tooltip}
            style={{
              position: 'fixed',
              left: position.x,
              top: position.y,
              background: tooltipBg,
              color: tooltipColor,
            }}
            initial={{ opacity: 0, y: 4 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 4 }}
            transition={{ duration: 0.15, ease: 'easeOut' }}
          >
            {content}
            <div
              className={styles.tooltipArrow}
              style={{ borderTopColor: tooltipBg }}
            />
          </motion.div>
        )}
      </AnimatePresence>
    </span>
  );
}
