import { useEffect, useRef } from 'react';
import { useLocation } from '@docusaurus/router';

const STORAGE_KEY = 'docs-sidebar-width';
const MIN_WIDTH = 200;
const MAX_WIDTH = 600;

/**
 * Headless component that makes the docs sidebar resizable by dragging its right edge.
 * Width is persisted in localStorage and restored on mount.
 */
export default function SidebarResizeHandler(): null {
  const location = useLocation();
  const cleanupRef = useRef<(() => void) | null>(null);

  // Restore persisted width once on mount.
  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const width = parseInt(stored, 10);
      if (!isNaN(width) && width >= MIN_WIDTH && width <= MAX_WIDTH) {
        document.documentElement.style.setProperty('--doc-sidebar-width', `${width}px`);
      }
    }
  }, []);

  // Attach drag handler on each route change (sidebar DOM may remount).
  useEffect(() => {
    cleanupRef.current?.();
    cleanupRef.current = null;

    const timeoutId = setTimeout(() => {
      const sidebar = document.querySelector<HTMLElement>('.theme-doc-sidebar-container');
      if (!sidebar) return;

      let startX = 0;
      let startWidth = 0;
      let dragging = false;

      const onMouseDown = (e: MouseEvent) => {
        const rect = sidebar.getBoundingClientRect();
        // Only trigger on the rightmost 8px of the sidebar.
        if (e.clientX < rect.right - 8) return;
        dragging = true;
        startX = e.clientX;
        startWidth = rect.width;
        document.documentElement.classList.add('sidebar-resizing');
        e.preventDefault();
      };

      const onMouseMove = (e: MouseEvent) => {
        if (!dragging) return;
        const newWidth = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, startWidth + (e.clientX - startX)));
        document.documentElement.style.setProperty('--doc-sidebar-width', `${newWidth}px`);
      };

      const onMouseUp = () => {
        if (!dragging) return;
        dragging = false;
        document.documentElement.classList.remove('sidebar-resizing');
        const current = getComputedStyle(document.documentElement)
          .getPropertyValue('--doc-sidebar-width')
          .trim();
        localStorage.setItem(STORAGE_KEY, parseInt(current, 10).toString());
      };

      sidebar.addEventListener('mousedown', onMouseDown);
      document.addEventListener('mousemove', onMouseMove);
      document.addEventListener('mouseup', onMouseUp);

      cleanupRef.current = () => {
        sidebar.removeEventListener('mousedown', onMouseDown);
        document.removeEventListener('mousemove', onMouseMove);
        document.removeEventListener('mouseup', onMouseUp);
      };
    }, 150);

    return () => {
      clearTimeout(timeoutId);
      cleanupRef.current?.();
    };
  }, [location.pathname]);

  return null;
}
