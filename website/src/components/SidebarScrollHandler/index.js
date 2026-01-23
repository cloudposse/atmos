import { useEffect } from 'react';
import { useLocation } from '@docusaurus/router';

/**
 * Automatically scrolls the sidebar to show the active item after navigation.
 * This fixes the issue where navigating to a different section (e.g., design patterns)
 * causes the sidebar to collapse/expand but doesn't scroll to show the active link.
 */
export default function SidebarScrollHandler() {
  const location = useLocation();

  useEffect(() => {
    // Wait for sidebar DOM to update after category collapse/expand.
    const timeoutId = setTimeout(() => {
      // Get all active links - there may be multiple (parent categories + actual page).
      // Filter to only visible links (non-zero dimensions) and pick the last/deepest one.
      const allActiveLinks = document.querySelectorAll('.menu__link--active');

      // Filter to only visible links (those with actual dimensions on screen).
      const visibleActiveLinks = Array.from(allActiveLinks).filter((link) => {
        const rect = link.getBoundingClientRect();
        return rect.width > 0 && rect.height > 0;
      });

      if (visibleActiveLinks.length === 0) return;

      const activeLink = visibleActiveLinks[visibleActiveLinks.length - 1];

      // Find the sidebar container that has overflow scrolling.
      const sidebarContainer = activeLink.closest('.theme-doc-sidebar-container');
      if (!sidebarContainer) return;

      // Get the scrollable menu within the sidebar.
      const scrollableMenu = sidebarContainer.querySelector('.menu') || sidebarContainer;

      // Calculate scroll position to center the active item in the sidebar viewport.
      const containerRect = scrollableMenu.getBoundingClientRect();
      const activeRect = activeLink.getBoundingClientRect();
      const offsetTop = activeRect.top - containerRect.top + scrollableMenu.scrollTop;
      const containerHeight = containerRect.height;

      // Scroll so the active item is vertically centered in the sidebar.
      scrollableMenu.scrollTo({
        top: Math.max(0, offsetTop - containerHeight / 2 + activeRect.height / 2),
        behavior: 'smooth',
      });
    }, 150); // Small delay to allow Docusaurus to update sidebar DOM.

    return () => clearTimeout(timeoutId);
  }, [location.pathname]); // Re-run on route changes.

  return null; // No UI to render.
}
