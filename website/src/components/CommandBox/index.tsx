import React, { useState } from 'react';
import './index.css';

interface CommandBoxProps {
  label: string;
  command: string;
}

export default function CommandBox({ label, command }: CommandBoxProps) {
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setFailed(true);
      setTimeout(() => setFailed(false), 2000);
    }
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
          title={failed ? 'Copy failed' : copied ? 'Copied!' : 'Copy to clipboard'}
        >
          {copied ? (
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
