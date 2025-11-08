import React, { useEffect, useState } from 'react';
import styles from './styles.module.css';

interface ThemeCredits {
  name: string;
  link: string;
}

interface ThemeMeta {
  isDark: boolean;
  recommended?: boolean;
  credits?: ThemeCredits[];
}

interface Theme {
  name: string;
  black: string;
  red: string;
  green: string;
  yellow: string;
  blue: string;
  magenta: string;
  cyan: string;
  white: string;
  brightBlack: string;
  brightRed: string;
  brightGreen: string;
  brightYellow: string;
  brightBlue: string;
  brightMagenta: string;
  brightCyan: string;
  brightWhite: string;
  background: string;
  foreground: string;
  cursor: string;
  selection: string;
  meta: ThemeMeta;
}

export default function ThemeGallery({ recommended = false }: { recommended?: boolean }) {
  const [themes, setThemes] = useState<Theme[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch('/themes.json')
      .then(res => res.json())
      .then((data: Theme[]) => {
        const filteredThemes = recommended
          ? data.filter(theme => theme.meta.recommended)
          : data;
        setThemes(filteredThemes);
        setLoading(false);
      })
      .catch(err => {
        setError(err.message);
        setLoading(false);
      });
  }, [recommended]);

  if (loading) {
    return <div className={styles.loading}>Loading themes...</div>;
  }

  if (error) {
    return <div className={styles.error}>Error loading themes: {error}</div>;
  }

  return (
    <div className={styles.gallery}>
      {themes.map(theme => (
        <div key={theme.name} className={styles.themeCard}>
          <div className={styles.themeHeader}>
            <h3 className={styles.themeName}>
              {theme.name}
              {theme.meta.recommended && <span className={styles.recommended}>★</span>}
            </h3>
            <div className={styles.themeMeta}>
              <span className={`${styles.badge} ${theme.meta.isDark ? styles.dark : styles.light}`}>
                {theme.meta.isDark ? 'Dark' : 'Light'}
              </span>
            </div>
          </div>

          <div className={styles.colorPalette}>
            <div
              className={styles.colorSwatch}
              style={{ backgroundColor: theme.background }}
              title={`Background: ${theme.background}`}
            />
            <div
              className={styles.colorSwatch}
              style={{ backgroundColor: theme.foreground }}
              title={`Foreground: ${theme.foreground}`}
            />
            <div
              className={styles.colorSwatch}
              style={{ backgroundColor: theme.red }}
              title={`Red: ${theme.red}`}
            />
            <div
              className={styles.colorSwatch}
              style={{ backgroundColor: theme.green }}
              title={`Green: ${theme.green}`}
            />
            <div
              className={styles.colorSwatch}
              style={{ backgroundColor: theme.yellow }}
              title={`Yellow: ${theme.yellow}`}
            />
            <div
              className={styles.colorSwatch}
              style={{ backgroundColor: theme.blue }}
              title={`Blue: ${theme.blue}`}
            />
            <div
              className={styles.colorSwatch}
              style={{ backgroundColor: theme.magenta }}
              title={`Magenta: ${theme.magenta}`}
            />
            <div
              className={styles.colorSwatch}
              style={{ backgroundColor: theme.cyan }}
              title={`Cyan: ${theme.cyan}`}
            />
          </div>

          <div className={styles.themeFooter}>
            <code className={styles.themeCommand}>
              ATMOS_THEME={theme.name.toLowerCase().replace(/\s+/g, '-')}
            </code>
            {theme.meta.credits && theme.meta.credits.length > 0 && (
              <div className={styles.credits}>
                Credits: {theme.meta.credits.map((credit, idx) => (
                  <React.Fragment key={idx}>
                    {idx > 0 && ', '}
                    <a href={credit.link} target="_blank" rel="noopener noreferrer">
                      {credit.name}
                    </a>
                  </React.Fragment>
                ))}
              </div>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
