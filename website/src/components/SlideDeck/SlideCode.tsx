import React from 'react';
import CodeBlock from '@theme/CodeBlock';
import type { SlideCodeProps } from './types';

export function SlideCode({
  children,
  language = 'yaml',
  showLineNumbers = false,
  className = '',
}: SlideCodeProps) {
  return (
    <div className={`slide-code ${className}`}>
      <CodeBlock language={language} showLineNumbers={showLineNumbers}>
        {children}
      </CodeBlock>
    </div>
  );
}

export default SlideCode;
