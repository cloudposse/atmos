import clsx from 'clsx';
import React from 'react';

export default function NavbarSeparator(props) {
  return <div {...props} className={clsx(props.className, 'separator')}/>;
}
