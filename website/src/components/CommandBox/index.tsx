import React, { useRef, useState } from 'react';
import { performCopy } from './copy.mjs';
import './index.css';

interface CommandBoxProps {
  label: string;
  command: string;
}

type CopyStatus = 'idle' | 'copied' | 'failed';

export default function CommandBox({ label, command }: CommandBoxProps) {
  const [status, setStatus] = useState<CopyStatus>('idle');
  const resetTimeoutRef = useRef<ReturnType<typeof setTimeout>>();

  const handleCopy = async () => {
    // A single status value keeps "copied" and "failed" mutually exclusive by
    // construction - a retry can never leave the button showing both at once.
    clearTimeout(resetTimeoutRef.current);
    const result = await performCopy((text) => navigator.clipboard.writeText(text), command);
    setStatus(result);
    resetTimeoutRef.current = setTimeout(() => setStatus('idle'), 2000);
  };

  return (
    <div className="command-box">
      <div className="command-box__content">
        <div className="command-box__label-wrapper">
          <span className="command-box__label">{label}</span>
        </div>
        <div className="command-box__command">
          <code className="command-box__code">{command}</code>
        </div>
        <button
          className="command-box__copy"
          onClick={handleCopy}
          title={status === 'failed' ? 'Copy failed' : status === 'copied' ? 'Copied!' : 'Copy to clipboard'}
        >
          {status === 'copied' ? (
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
              <path d="M13.5 4L6 11.5L2.5 8" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
            </svg>
          ) : (
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
              <rect x="5" y="5" width="9" height="9" rx="1" stroke="currentColor" strokeWidth="1.5"/>
              <path d="M3 11V3C3 2.44772 3.44772 2 4 2H11" stroke="currentColor" strokeWidth="1.5"/>
            </svg>
          )}
        </button>
      </div>
    </div>
  );
}
