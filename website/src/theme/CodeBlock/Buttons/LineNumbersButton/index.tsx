import React, { useEffect, useState } from 'react';
import clsx from 'clsx';
import { translate } from '@docusaurus/Translate';
import { useCodeBlockContext } from '@docusaurus/theme-common/internal';
import Button from '@theme/CodeBlock/Buttons/Button';
import { FiHash } from 'react-icons/fi';

const STORAGE_KEY = 'atmos-code-line-numbers';
const CHANGE_EVENT = 'atmos-code-line-numbers-change';

type LineNumberPreference = 'on' | 'off';

function readPreference(): LineNumberPreference {
  if (typeof window === 'undefined') {
    return 'on';
  }

  return window.localStorage.getItem(STORAGE_KEY) === 'off' ? 'off' : 'on';
}

function applyPreference(preference: LineNumberPreference): void {
  document.documentElement.dataset.codeLineNumbers = preference;
  window.localStorage.setItem(STORAGE_KEY, preference);
  window.dispatchEvent(new CustomEvent(CHANGE_EVENT, { detail: { preference } }));
}

export default function LineNumbersButton({ className }: { className?: string }): JSX.Element | false {
  const { metadata } = useCodeBlockContext();
  const [preference, setPreference] = useState<LineNumberPreference>(() => readPreference());
  const canToggleLineNumbers = metadata.lineNumbersStart !== undefined;

  useEffect(() => {
    applyPreference(readPreference());

    function handlePreferenceChange(event: Event) {
      const customEvent = event as CustomEvent<{ preference?: LineNumberPreference }>;
      setPreference(customEvent.detail?.preference === 'off' ? 'off' : 'on');
    }

    function handleStorage(event: StorageEvent) {
      if (event.key === STORAGE_KEY) {
        setPreference(readPreference());
        applyPreference(readPreference());
      }
    }

    window.addEventListener(CHANGE_EVENT, handlePreferenceChange);
    window.addEventListener('storage', handleStorage);
    return () => {
      window.removeEventListener(CHANGE_EVENT, handlePreferenceChange);
      window.removeEventListener('storage', handleStorage);
    };
  }, []);

  if (!canToggleLineNumbers) {
    return false;
  }

  const lineNumbersEnabled = preference !== 'off';
  const title = lineNumbersEnabled
    ? translate({
        id: 'theme.CodeBlock.lineNumbersHide',
        message: 'Hide line numbers',
        description: 'The title attribute for the button that hides code block line numbers',
      })
    : translate({
        id: 'theme.CodeBlock.lineNumbersShow',
        message: 'Show line numbers',
        description: 'The title attribute for the button that shows code block line numbers',
      });

  return (
    <Button
      onClick={() => {
        const nextPreference = lineNumbersEnabled ? 'off' : 'on';
        applyPreference(nextPreference);
        setPreference(nextPreference);
      }}
      className={clsx(className, lineNumbersEnabled && 'code-line-numbers-button--enabled')}
      aria-pressed={lineNumbersEnabled}
      aria-label={title}
      title={title}
    >
      <FiHash className="code-line-numbers-button__icon" aria-hidden="true" />
    </Button>
  );
}
