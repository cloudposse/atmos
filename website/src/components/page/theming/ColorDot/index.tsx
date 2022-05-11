import clsx from 'clsx';
import React from 'react';

import useThemeContext from '@theme/hooks/useThemeContext';

import styles from './index.module.scss';

export default function ColorDot({ color, ...props }) {
  const { isDarkTheme } = useThemeContext();

  return (
    <div
      style={{ backgroundColor: color }}
      className={clsx(
        props.className,
        'color-dot',
        styles.colorDot,
        styles[`colorDot${isDarkTheme ? 'Dark' : 'Light'}`]
      )}
      {...props}
    />
  );
}
