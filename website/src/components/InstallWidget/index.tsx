import React, { useState } from 'react';
import useGlobalData from '@docusaurus/useGlobalData';
import './index.css';

interface InstallMethod {
  label: string;
  command: string;
  language: string;
}

export default function InstallWidget() {
  const globalData = useGlobalData();
  const latestRelease = globalData['fetch-latest-release']?.default?.latestRelease || 'latest';
  // Remove 'v' prefix for Docker tag (e.g., v1.197.0 -> 1.197.0)
  const dockerTag = latestRelease.startsWith('v') ? latestRelease.slice(1) : latestRelease;

  const installMethods: InstallMethod[] = [
    { label: 'Quick Install', command: 'curl -fsSL https://atmos.tools/install.sh | bash', language: 'bash' },
    { label: 'macOS', command: 'brew install atmos', language: 'bash' },
    { label: 'Windows', command: 'choco install atmos', language: 'powershell' },
    { label: 'Docker', command: `docker pull ghcr.io/cloudposse/atmos:${dockerTag}`, language: 'bash' },
  ];

  const [selectedMethod, setSelectedMethod] = useState(0);
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(installMethods[selectedMethod].command);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const renderCommand = () => {
    const command = installMethods[selectedMethod].command;

    // Syntax highlighting for the curl command
    if (command.startsWith('curl -fsSL')) {
      const parts = command.split(' ');
      const url = parts.slice(2).join(' ').replace(/\s*\|\s*bash$/, '');
      return (
        <>
          <span className="install-widget__command-curl">curl -fsSL</span>
          {' '}
          <span className="install-widget__command-url">{url}</span>
          {' | '}
          <span className="install-widget__command-bash">bash</span>
        </>
      );
    }

    // For other commands, just return plain text
    return command;
  };

  return (
    <div className="install-widget">
      <div className="install-widget__content">
        <div className="install-widget__select-wrapper">
          <span className="install-widget__label">Terminal</span>
          <select
            className="install-widget__select"
            value={selectedMethod}
            onChange={(e) => setSelectedMethod(Number(e.target.value))}
          >
            {installMethods.map((method, index) => (
              <option key={index} value={index}>
                {method.label}
              </option>
            ))}
          </select>
          <svg className="install-widget__chevron" width="12" height="8" viewBox="0 0 12 8" fill="none">
            <path d="M1 1.5L6 6.5L11 1.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
          </svg>
        </div>
        <div className="install-widget__command">
          <code className="install-widget__code">
            {renderCommand()}
          </code>
        </div>
        <button
          className="install-widget__copy"
          onClick={handleCopy}
          title={copied ? 'Copied!' : 'Copy to clipboard'}
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
