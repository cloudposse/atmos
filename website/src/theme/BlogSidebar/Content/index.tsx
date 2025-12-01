/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
import React, {memo, useState} from 'react';
import useGlobalData from '@docusaurus/useGlobalData';
import type {BlogSidebarItem} from '@docusaurus/plugin-content-blog';
import Heading from '@theme/Heading';
import {compareVersionsDescending} from '@site/src/components/ChangelogTimeline/utils';

interface ReleaseGroup {
  release: string;
  items: BlogSidebarItem[];
}

/**
 * Groups blog sidebar items by release version using the releaseMap from global data.
 * Posts without a release are grouped under 'unreleased'.
 */
function groupBlogSidebarItemsByRelease(
  items: BlogSidebarItem[],
  releaseMap: Record<string, string>,
): ReleaseGroup[] {
  const releaseGroups = new Map<string, BlogSidebarItem[]>();

  items.forEach((item) => {
    const release = releaseMap[item.permalink] || 'unreleased';
    if (!releaseGroups.has(release)) {
      releaseGroups.set(release, []);
    }
    releaseGroups.get(release)!.push(item);
  });

  // Sort releases: unreleased first, then by version descending.
  const sortedReleases = Array.from(releaseGroups.keys()).sort(compareVersionsDescending);

  return sortedReleases.map((release) => ({
    release,
    items: releaseGroups.get(release)!,
  }));
}

function CollapsibleReleaseGroup({
  release,
  children,
  isDefaultOpen,
  releaseGroupHeadingClassName,
}: {
  release: string;
  children: React.ReactNode;
  isDefaultOpen: boolean;
  releaseGroupHeadingClassName?: string;
}) {
  const [isOpen, setIsOpen] = useState(isDefaultOpen);
  const isUnreleased = release === 'unreleased';
  const displayRelease = isUnreleased ? 'Unreleased' : release;

  return (
    <div role="group">
      <button
        onClick={() => setIsOpen(!isOpen)}
        style={{
          display: 'flex',
          alignItems: 'center',
          width: '100%',
          background: 'none',
          border: 'none',
          padding: '0.5rem 0',
          cursor: 'pointer',
          textAlign: 'left',
        }}
        aria-expanded={isOpen}
      >
        <svg
          viewBox="0 0 16 16"
          width="12"
          height="12"
          style={{
            marginRight: '0.5rem',
            transition: 'transform 0.2s',
            transform: isOpen ? 'rotate(90deg)' : 'rotate(0deg)',
          }}
        >
          <path
            fill="currentColor"
            d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z"
          />
        </svg>
        <Heading
          as="h4"
          className={releaseGroupHeadingClassName}
          style={{
            fontSize: '0.9rem',
            margin: 0,
            fontWeight: 600,
          }}
        >
          {displayRelease}
        </Heading>
        {!isUnreleased && (
          <a
            href={`https://github.com/cloudposse/atmos/releases/tag/${release}`}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            style={{
              marginLeft: '0.5rem',
              color: 'var(--ifm-color-emphasis-600)',
              display: 'flex',
              alignItems: 'center',
            }}
            title="View release on GitHub"
          >
            <svg viewBox="0 0 16 16" width="14" height="14">
              <path
                fill="currentColor"
                d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"
              />
            </svg>
          </a>
        )}
      </button>
      {isOpen && <div>{children}</div>}
    </div>
  );
}

function BlogSidebarContent({items, yearGroupHeadingClassName, ListComponent}: {
  items: BlogSidebarItem[];
  yearGroupHeadingClassName?: string;
  ListComponent: React.ComponentType<{items: BlogSidebarItem[]}>;
}) {
  const globalData = useGlobalData();
  const releaseData = globalData['blog-release-data']?.default as {releaseMap: Record<string, string>} | undefined;
  const releaseMap = releaseData?.releaseMap || {};

  const groupedByRelease = groupBlogSidebarItemsByRelease(items, releaseMap);

  return (
    <>
      {groupedByRelease.map((group, index) => (
        <CollapsibleReleaseGroup
          key={group.release}
          release={group.release}
          isDefaultOpen={index === 0}
          releaseGroupHeadingClassName={yearGroupHeadingClassName}
        >
          <ListComponent items={group.items} />
        </CollapsibleReleaseGroup>
      ))}
    </>
  );
}

export default memo(BlogSidebarContent);
