import React from 'react';
import OriginalCodeBlock from '@theme-original/CodeBlock';
import Terminal, { useIsInsideTerminal } from '@site/src/components/Terminal';

const OUTPUT_LANGUAGES = new Set(['ansi', 'console', 'output', 'terminal']);

function getLanguage(props: Record<string, unknown>): string {
  if (typeof props.language === 'string') {
    return props.language.toLowerCase();
  }

  if (typeof props.className === 'string') {
    return props.className.match(/language-([\w-]+)/)?.[1]?.toLowerCase() ?? '';
  }

  return '';
}

function getTerminalTitle(props: Record<string, unknown>): string {
  if (typeof props.title === 'string' && props.title.trim()) {
    return props.title;
  }

  return 'Terminal';
}

export default function CodeBlock(props: Record<string, unknown>): JSX.Element {
  const language = getLanguage(props);
  const isInsideTerminal = useIsInsideTerminal();

  if (OUTPUT_LANGUAGES.has(language) && !isInsideTerminal) {
    const { title, ...codeBlockProps } = props;

    return (
      <Terminal title={getTerminalTitle(props)}>
        <OriginalCodeBlock {...codeBlockProps} />
      </Terminal>
    );
  }

  return <OriginalCodeBlock {...props} />;
}
