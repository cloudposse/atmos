import React, { useEffect, useState } from 'react';

export default function CloudPosseSlackEmbed(): JSX.Element {
  const [theme, setTheme] = useState<'light' | 'dark'>('dark');

  useEffect(() => {
    const checkTheme = () => {
      const currentTheme = document.documentElement.getAttribute('data-theme');
      setTheme(currentTheme === 'dark' ? 'dark' : 'light');
    };

    checkTheme();

    // Watch for theme changes
    const observer = new MutationObserver(checkTheme);
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    });

    return () => observer.disconnect();
  }, []);

  const src = `https://cloudposse.com/embed/slack?theme=${theme}&bg=transparent&utm_source=atmos-docs&utm_medium=embed&utm_campaign=slack-community&utm_content=community-page`;

  return (
    <iframe
      src={src}
      style={{
        height: '380px',
        width: '100%',
        maxWidth: '80rem',
        borderRadius: '0.375rem',
        border: '0',
      }}
      title="Join our Slack Community"
      sandbox="allow-same-origin allow-scripts allow-forms allow-popups"
    />
  );
}
