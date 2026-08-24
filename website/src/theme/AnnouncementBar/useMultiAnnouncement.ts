import {useState, useEffect, useCallback, useMemo} from 'react';
import useIsBrowser from '@docusaurus/useIsBrowser';
import {announcements, dismissCooldownMs} from '@site/src/data/announcements';

const STORAGE_KEY = 'atmos.announcements.dismissed';
const LAST_DISMISSED_KEY = 'atmos.announcements.lastDismissedAt';

interface DismissState {
  ids: string[];
  lastDismissedAt: number | null;
}

function readState(): DismissState {
  try {
    const rawIds = localStorage.getItem(STORAGE_KEY);
    const rawTime = localStorage.getItem(LAST_DISMISSED_KEY);
    const parsedIds: unknown = rawIds ? JSON.parse(rawIds) : [];
    const ids = Array.isArray(parsedIds)
      ? parsedIds.filter((id): id is string => typeof id === 'string')
      : [];
    const parsedTime = rawTime ? Number(rawTime) : null;
    const lastDismissedAt =
      parsedTime !== null && Number.isFinite(parsedTime) ? parsedTime : null;
    return {
      ids,
      lastDismissedAt,
    };
  } catch {
    // localStorage unavailable or corrupted.
  }
  return {ids: [], lastDismissedAt: null};
}

function writeState(ids: string[], lastDismissedAt: number): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(ids));
    localStorage.setItem(LAST_DISMISSED_KEY, String(lastDismissedAt));
  } catch {
    // localStorage unavailable.
  }
}

function isCoolingDown(lastDismissedAt: number | null): boolean {
  if (lastDismissedAt === null) return false;
  return Date.now() - lastDismissedAt < dismissCooldownMs;
}

export function useMultiAnnouncement() {
  const isBrowser = useIsBrowser();

  const [dismissedIds, setDismissedIds] = useState<Set<string>>(() => {
    if (isBrowser) {
      return new Set(readState().ids);
    }
    return new Set();
  });

  const [lastDismissedAt, setLastDismissedAt] = useState<number | null>(() => {
    if (isBrowser) {
      return readState().lastDismissedAt;
    }
    return null;
  });

  const [coolingDown, setCoolingDown] = useState<boolean>(() => {
    if (isBrowser) {
      return isCoolingDown(readState().lastDismissedAt);
    }
    return false;
  });

  // Start hidden during SSR to prevent flash-of-content when bar should be
  // suppressed.  Flips to true after client hydration reads localStorage.
  const [hydrated, setHydrated] = useState<boolean>(isBrowser);

  // Hydrate on mount (covers SSR -> client transition).
  useEffect(() => {
    const state = readState();
    setDismissedIds(new Set(state.ids));
    setLastDismissedAt(state.lastDismissedAt);
    setCoolingDown(isCoolingDown(state.lastDismissedAt));
    setHydrated(true);
  }, []);

  // Set a timer to end the cooldown period once it expires.
  useEffect(() => {
    if (lastDismissedAt === null || !coolingDown) return;
    const remaining = dismissCooldownMs - (Date.now() - lastDismissedAt);
    if (remaining <= 0) {
      setCoolingDown(false);
      return;
    }
    const timer = setTimeout(() => setCoolingDown(false), remaining);
    return () => clearTimeout(timer);
  }, [lastDismissedAt, coolingDown]);

  const nextAnnouncement = useMemo(
    () => announcements.find((a) => !dismissedIds.has(a.id)) ?? null,
    [dismissedIds],
  );

  const allDismissed = nextAnnouncement === null;
  // Bar is hidden until hydrated, when cooling down, or when all dismissed.
  const isActive = hydrated && !allDismissed && !coolingDown;

  const dismiss = useCallback(() => {
    if (!nextAnnouncement) return;
    const now = Date.now();
    setDismissedIds((prev) => {
      const next = new Set(prev);
      next.add(nextAnnouncement.id);
      writeState([...next], now);
      return next;
    });
    setLastDismissedAt(now);
    setCoolingDown(true);
  }, [nextAnnouncement]);

  return useMemo(
    () => ({
      current: isActive ? nextAnnouncement : null,
      dismiss,
      isActive,
    }),
    [nextAnnouncement, dismiss, isActive],
  );
}
