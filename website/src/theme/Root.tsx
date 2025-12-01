import React from 'react';
import Watermark from '@site/src/components/Watermark';

/**
 * Root component that wraps the entire site.
 * Used to add global elements like the watermark.
 */
export default function Root({ children }: { children: React.ReactNode }): JSX.Element {
  return (
    <>
      {children}
      <Watermark />
    </>
  );
}
