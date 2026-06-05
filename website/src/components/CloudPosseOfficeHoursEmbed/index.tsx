import React, { useEffect, useState } from 'react';

export default function CloudPosseOfficeHoursEmbed(): JSX.Element {
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

  const src = `https://cloudposse.com/embed/office-hours?theme=${theme}&bg=transparent&utm_source=atmos-docs&utm_medium=embed&utm_campaign=office-hours&utm_content=community-page`;

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
      title="Office Hours Registration"
      sandbox="allow-same-origin allow-scripts allow-forms allow-popups"
    />
  );
}
