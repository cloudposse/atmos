import React from 'react';
import clsx from 'clsx';
import BrowserOnly from '@docusaurus/BrowserOnly';
import CopyButton from '@theme/CodeBlock/Buttons/CopyButton';
import LineNumbersButton from '@theme/CodeBlock/Buttons/LineNumbersButton';
import WordWrapButton from '@theme/CodeBlock/Buttons/WordWrapButton';
import styles from './styles.module.css';

export default function CodeBlockButtons({ className }: { className?: string }): JSX.Element {
  return (
    <BrowserOnly>
      {() => (
        <div className={clsx(className, styles.buttonGroup)}>
          <LineNumbersButton />
          <WordWrapButton />
          <CopyButton />
        </div>
      )}
    </BrowserOnly>
  );
}
