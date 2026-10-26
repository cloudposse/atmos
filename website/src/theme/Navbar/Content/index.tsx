import React, { type ReactNode, useRef, useCallback } from 'react';
import clsx from 'clsx';
import {
  useThemeConfig,
  ErrorCauseBoundary,
  ThemeClassNames,
} from '@docusaurus/theme-common';
import {
  splitNavbarItems,
  useNavbarMobileSidebar,
} from '@docusaurus/theme-common/internal';
import NavbarItem, { type Props as NavbarItemConfig } from '@theme/NavbarItem';
import NavbarColorModeToggle from '@theme/Navbar/ColorModeToggle';
import SearchBar from '@theme/SearchBar';
import NavbarMobileSidebarToggle from '@theme/Navbar/MobileSidebar/Toggle';
import NavbarLogo from '@theme/Navbar/Logo';
import NavbarSearch from '@theme/Navbar/Search';

import { usePriorityNavbar } from './usePriorityNavbar';
import styles from './styles.module.css';

function useNavbarItems() {
  // TODO temporary casting until ThemeConfig type is improved.
  return useThemeConfig().navbar.items as NavbarItemConfig[];
}

interface NavbarItemWithRefProps {
  item: NavbarItemConfig;
  index: number;
  onRef: (index: number, el: HTMLElement | null) => void;
  isVisible: boolean;
}

function NavbarItemWithRef({ item, index, onRef, isVisible }: NavbarItemWithRefProps): ReactNode {
  return (
    <div
      ref={(el) => onRef(index, el)}
      className={clsx(
        styles.priorityItem,
        !isVisible && styles.priorityItemHidden
      )}
    >
      <ErrorCauseBoundary
        onError={(error) =>
          new Error(
            `A theme navbar item failed to render.
Please double-check the following navbar item (themeConfig.navbar.items) of your Docusaurus config:
${JSON.stringify(item, null, 2)}`,
            { cause: error },
          )
        }>
        <NavbarItem {...item} />
      </ErrorCauseBoundary>
    </div>
  );
}

function NavbarContentLayout({
  left,
  right,
}: {
  left: ReactNode;
  right: ReactNode;
}) {
  return (
    <div className="navbar__inner">
      <div
        className={clsx(
          ThemeClassNames.layout.navbar.containerLeft,
          'navbar__items',
        )}>
        {left}
      </div>
      <div
        className={clsx(
          ThemeClassNames.layout.navbar.containerRight,
          'navbar__items navbar__items--right',
        )}>
        {right}
      </div>
    </div>
  );
}

export default function NavbarContent(): ReactNode {
  const mobileSidebar = useNavbarMobileSidebar();
  const items = useNavbarItems();
  const [leftItems, rightItems] = splitNavbarItems(items);
  const searchBarItem = items.find((item) => item.type === 'search');

  // Priority navigation state.
  const containerRef = useRef<HTMLDivElement>(null);
  const itemRefsArray = useRef<(HTMLElement | null)[]>([]);

  // Callback to collect item refs.
  const handleItemRef = useCallback((index: number, el: HTMLElement | null) => {
    itemRefsArray.current[index] = el;
  }, []);

  // Use priority navigation hook.
  const { visibleCount, hasOverflow, isMobile, isReady } = usePriorityNavbar(
    containerRef,
    itemRefsArray,
    leftItems.length,
    { toggleWidth: 48, gap: 8 }
  );

  return (
    <NavbarContentLayout
      left={
        <>
          <NavbarLogo />
          <div
            ref={containerRef}
            className={clsx(
              styles.priorityContainer,
              !isReady && styles.priorityContainerMeasuring
            )}
          >
            {leftItems.map((item, i) => (
              <NavbarItemWithRef
                key={i}
                item={item}
                index={i}
                onRef={handleItemRef}
                isVisible={!isReady || i < visibleCount}
              />
            ))}
          </div>
        </>
      }
      right={
        <>
          <NavbarItems items={rightItems} />
          <NavbarColorModeToggle className={styles.colorModeToggle} />
          {!searchBarItem && (
            <NavbarSearch>
              <SearchBar />
            </NavbarSearch>
          )}
          {/* Mobile sidebar toggle - shows when items overflow or in mobile mode. */}
          {!mobileSidebar.disabled && (hasOverflow || isMobile) && (
            <div className={styles.sidebarToggle}>
              <NavbarMobileSidebarToggle />
            </div>
          )}
        </>
      }
    />
  );
}

// Helper component for right-side items that don't need priority handling.
function NavbarItems({ items }: { items: NavbarItemConfig[] }): ReactNode {
  return (
    <>
      {items.map((item, i) => (
        <ErrorCauseBoundary
          key={i}
          onError={(error) =>
            new Error(
              `A theme navbar item failed to render.
Please double-check the following navbar item (themeConfig.navbar.items) of your Docusaurus config:
${JSON.stringify(item, null, 2)}`,
              { cause: error },
            )
          }>
          <NavbarItem {...item} />
        </ErrorCauseBoundary>
      ))}
    </>
  );
}
