import { useEffect } from 'react';

const STORAGE_KEY = 'atmos-code-line-numbers';

export default function CodeLineNumberPreference(): null {
  useEffect(() => {
    const preference = window.localStorage.getItem(STORAGE_KEY) === 'off' ? 'off' : 'on';
    document.documentElement.dataset.codeLineNumbers = preference;
  }, []);

  return null;
}
