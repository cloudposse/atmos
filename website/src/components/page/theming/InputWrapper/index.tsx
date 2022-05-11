import clsx from 'clsx';
import React from 'react';

import styles from './index.module.scss';

import useThemeContext from '@theme/hooks/useThemeContext';

export default function InputWrapper({ ...props }) {
  const { isDarkTheme } = useThemeContext();

  return (
    <div
      {...props}
      className={clsx(
        props.className,
        'input-wrapper',
        styles.inputWrapper,
        styles[`inputWrapper${isDarkTheme ? 'Dark' : 'Light'}`]
      )}
    />
  );
}
