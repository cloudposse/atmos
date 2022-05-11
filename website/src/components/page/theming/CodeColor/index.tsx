import clsx from 'clsx';
import React from 'react';

import styles from './index.module.scss';

function CodeColor({ color, ...props }): JSX.Element {
  return (
    <div className={clsx(styles.codeColor, props.className, 'code-color')}>
      <span
        className={styles.codeColorBlock}
        style={{
          backgroundColor: color,
          ...props.style,
        }}
      />
      {props.children && <code className={styles.codeColorValue}>{props.children}</code>}
    </div>
  );
}

export default CodeColor;
