import React, {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useSyncExternalStore,
} from "react";
import { useHistory } from "@docusaurus/router";
import { createNavigationStore, historyStateKey } from "./state.mjs";

type NavigationContextValue = {
  entry?: string;
  expanded: Record<string, boolean>;
  filtering: boolean;
  toggle: (id: string, expanded: boolean) => void;
};
export const NavigationContext = createContext<NavigationContextValue | null>(
  null,
);
export const useNavigationContext = () => useContext(NavigationContext);

const stores = new WeakMap<object, ReturnType<typeof createNavigationStore>>();
const serverSnapshot = () => null;

/** Share history-backed navigation while using canonical markup during hydration. */
export function useNavigationStore(items, sidebar: string) {
  const history = useHistory();
  const store = useMemo(() => {
    if (typeof window === "undefined") return createNavigationStore(history);
    let existing = stores.get(history);
    if (!existing) {
      existing = createNavigationStore(history, {
        read: () => window.history.state?.state?.[historyStateKey],
        // Preserve the router key and unrelated state. Updating only the current
        // entry avoids replacing its identity or triggering a route transition.
        write: (snapshot) =>
          window.history.replaceState(
            {
              ...window.history.state,
              state: {
                ...window.history.state?.state,
                [historyStateKey]: snapshot,
              },
            },
            "",
          ),
      });
      stores.set(history, existing);
    }
    return existing;
  }, [history]);
  const snapshot = useSyncExternalStore(
    store.subscribe,
    store.getSnapshot,
    serverSnapshot,
  );
  useEffect(() => store.register(items, sidebar), [store, items, sidebar]);
  return { store, snapshot };
}
