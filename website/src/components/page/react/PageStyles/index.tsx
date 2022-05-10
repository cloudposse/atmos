import clsx from 'clsx';
import React from 'react';
import styles from './index.module.scss';

export default function PageStyles(props) {
  return <div {...props} className={clsx(styles.pageReact, props.className)}/>;
}
