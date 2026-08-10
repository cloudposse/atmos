import React from 'react';
import OriginalCodeBlockString from '@theme-original/CodeBlock/Content/String';

const TERMINAL_LANGUAGES = new Set([
  'ansi',
  'bash',
  'console',
  'output',
  'powershell',
  'ps1',
  'sh',
  'shell',
  'terminal',
  'text',
  'txt',
]);

function getLanguage(props: Record<string, unknown>): string {
  if (typeof props.language === 'string') {
    return props.language.toLowerCase();
  }

  if (typeof props.className === 'string') {
    return props.className.match(/language-([\w-]+)/)?.[1]?.toLowerCase() ?? '';
  }

  return '';
}

function hasMultipleLines(children: unknown): boolean {
  return typeof children === 'string' && children.trimEnd().includes('\n');
}

export default function CodeBlockString(props: Record<string, unknown>): JSX.Element {
  const language = getLanguage(props);
  const shouldShowLineNumbers =
    props.showLineNumbers ?? (hasMultipleLines(props.children) && !TERMINAL_LANGUAGES.has(language));

  return (
    <OriginalCodeBlockString {...props} showLineNumbers={shouldShowLineNumbers}>
      {props.children}
    </OriginalCodeBlockString>
  );
}
