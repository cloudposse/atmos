/**
 * Context for sharing sidebar collapsed state between BlogSidebar and BlogLayout.
 */
import React, { createContext, useContext, useState, type ReactNode } from 'react';

interface SidebarContextValue {
  isCollapsed: boolean;
  setIsCollapsed: (collapsed: boolean) => void;
}

const SidebarContext = createContext<SidebarContextValue | null>(null);

export function SidebarProvider({ children }: { children: ReactNode }) {
  const [isCollapsed, setIsCollapsed] = useState(true);
  return (
    <SidebarContext.Provider value={{ isCollapsed, setIsCollapsed }}>
      {children}
    </SidebarContext.Provider>
  );
}

export function useSidebarCollapsed() {
  const context = useContext(SidebarContext);
  if (!context) {
    // Return default values if used outside provider
    return { isCollapsed: true, setIsCollapsed: () => {} };
  }
  return context;
}
