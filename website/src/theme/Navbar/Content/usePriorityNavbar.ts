import { useState, useEffect, useCallback, useRef, RefObject } from 'react';

interface UsePriorityNavbarOptions {
  // Extra width to reserve for the overflow toggle button.
  toggleWidth?: number;
  // Gap between items.
  gap?: number;
  // Minimum width before switching to full mobile mode.
  mobileBreakpoint?: number;
}

interface UsePriorityNavbarResult {
  // Number of items that fit in the visible navbar.
  visibleCount: number;
  // Whether any items overflow (determines if toggle should show).
  hasOverflow: boolean;
  // Whether we're in mobile mode (below mobileBreakpoint).
  isMobile: boolean;
  // Whether initial measurement is complete.
  isReady: boolean;
}

/**
 * Hook to measure navbar items and calculate how many fit in available space.
 * Returns the number of visible items and whether overflow exists.
 *
 * Uses cached measurements for hidden items to avoid measurement issues
 * when items are hidden with CSS.
 *
 * @param containerRef - Ref to the container element to measure.
 * @param itemRefs - Array of refs to each navbar item element.
 * @param totalItems - Total number of items to consider.
 * @param options - Configuration options.
 */
export function usePriorityNavbar(
  containerRef: RefObject<HTMLElement>,
  itemRefs: RefObject<(HTMLElement | null)[]>,
  totalItems: number,
  options: UsePriorityNavbarOptions = {}
): UsePriorityNavbarResult {
  const {
    toggleWidth = 48, // Width of hamburger button.
    gap = 0,
    mobileBreakpoint = 996,
  } = options;

  // Start with all items visible to allow measurement.
  const [visibleCount, setVisibleCount] = useState(totalItems);
  const [measuredItemCount, setMeasuredItemCount] = useState(totalItems);
  const [isMobile, setIsMobile] = useState(false);
  const [isReady, setIsReady] = useState(false);

  // Cache measured widths so we don't need to measure hidden items.
  const itemWidthsRef = useRef<number[]>([]);
  const rafRef = useRef<number | null>(null);

  const calculateVisibleItems = useCallback(() => {
    const container = containerRef.current;
    // Clamp refs to totalItems to avoid stale trailing entries when items are removed.
    // This prevents endless re-measure loops when itemRefs.current.length > totalItems.
    const items = Array.from(
      { length: totalItems },
      (_, i) => itemRefs.current?.[i] ?? null
    );

    if (!container || items.length === 0) {
      return;
    }

    // Check if we're in mobile mode.
    const viewportWidth = window.innerWidth;
    if (viewportWidth <= mobileBreakpoint) {
      setIsMobile(true);
      setVisibleCount(0);
      setIsReady(true);
      return;
    }

    setIsMobile(false);

    // Measure item widths on first pass (when all items are visible).
    // Cache them for subsequent calculations.
    // Re-measure if any items have zero width or are missing.
    const cachedNeedsInit =
      itemWidthsRef.current.length !== items.length ||
      itemWidthsRef.current.some((w) => w === 0) ||
      items.some((el) => !el);

    if (cachedNeedsInit) {
      const widths: number[] = [];
      let allMeasured = true;
      for (let i = 0; i < items.length; i++) {
        const item = items[i];
        if (item) {
          // Get scroll width for accurate measurement even if overflow:hidden.
          const w = item.scrollWidth || item.getBoundingClientRect().width;
          widths[i] = w;
          if (!w) allMeasured = false;
        } else {
          widths[i] = 0;
          allMeasured = false;
        }
      }
      itemWidthsRef.current = widths;

      // If we couldn't measure everything yet, try again next frame.
      if (!allMeasured) {
        setIsReady(false);
        if (rafRef.current) cancelAnimationFrame(rafRef.current);
        rafRef.current = requestAnimationFrame(calculateVisibleItems);
        return;
      }
    }

    // Get available width for left items.
    const containerRect = container.getBoundingClientRect();

    // Find the navbar__inner to get the full available width.
    const navbarInner = container.closest('.navbar__inner');
    const rightSection = navbarInner?.querySelector('.navbar__items--right');

    let availableWidth: number;
    if (navbarInner && rightSection) {
      const innerRect = navbarInner.getBoundingClientRect();
      const rightRect = rightSection.getBoundingClientRect();
      // Logo is before our container.
      const logoWidth = containerRect.left - innerRect.left;
      // Available = inner width - logo - right section - some padding.
      availableWidth = innerRect.width - logoWidth - rightRect.width - 32;
    } else {
      availableWidth = containerRect.width;
    }

    // Calculate how many items fit using cached widths (all non-zero now).
    const cachedWidths = itemWidthsRef.current;
    const measuredItems = cachedWidths.map((width, index) => ({ width, index }));
    const measuredCount = measuredItems.length;

    let totalWidth = 0;
    let fitCount = 0;

    for (let i = 0; i < measuredItems.length; i++) {
      const { width: itemWidth } = measuredItems[i];

      // Add gap for items after the first.
      const widthWithGap = fitCount > 0 ? itemWidth + gap : itemWidth;

      // If there will be overflow, reserve space for the toggle.
      const wouldOverflow = totalWidth + widthWithGap > availableWidth;
      const needsToggle = i < measuredCount - 1 && wouldOverflow;

      // Check if this item fits (with toggle space if needed).
      const spaceNeeded = needsToggle
        ? totalWidth + widthWithGap + toggleWidth
        : totalWidth + widthWithGap;

      if (spaceNeeded <= availableWidth) {
        totalWidth += widthWithGap;
        fitCount++;
      } else {
        // This item doesn't fit.
        // But we need to check if previous items + toggle fit.
        if (totalWidth + toggleWidth > availableWidth && fitCount > 0) {
          fitCount--; // Remove one more item to make room for toggle.
        }
        break;
      }
    }

    setVisibleCount(fitCount);
    setMeasuredItemCount(measuredCount);
    setIsReady(true);
  }, [containerRef, itemRefs, totalItems, toggleWidth, gap, mobileBreakpoint]);

  // Re-measure widths when items change.
  useEffect(() => {
    itemWidthsRef.current = [];
  }, [totalItems]);

  useEffect(() => {
    const handleResize = () => {
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
      }
      rafRef.current = requestAnimationFrame(calculateVisibleItems);
    };

    // Initial calculation after a short delay to ensure DOM is ready.
    const initialTimeout = setTimeout(() => {
      calculateVisibleItems();
    }, 50);

    window.addEventListener('resize', handleResize);

    // Use ResizeObserver for container-specific changes.
    const container = containerRef.current;
    let resizeObserver: ResizeObserver | null = null;

    if (container && typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(handleResize);
      resizeObserver.observe(container);

      // Also observe the navbar inner for better accuracy.
      const navbarInner = container.closest('.navbar__inner');
      if (navbarInner) {
        resizeObserver.observe(navbarInner);
      }
    }

    return () => {
      clearTimeout(initialTimeout);
      window.removeEventListener('resize', handleResize);
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
      }
      if (resizeObserver) {
        resizeObserver.disconnect();
      }
    };
  }, [calculateVisibleItems, containerRef]);

  return {
    visibleCount,
    hasOverflow: visibleCount < measuredItemCount && !isMobile,
    isMobile,
    isReady,
  };
}
