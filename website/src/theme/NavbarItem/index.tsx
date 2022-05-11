import OriginalNavbarItem from '@theme-original/NavbarItem';
import React from 'react';
import NavbarIconLink from '@theme/NavbarItem/NavbarIconLink';
import NavbarSeparator from '@theme/NavbarItem/NavbarSeparator';
import NavbarCta from '@theme/NavbarItem/NavbarCta';

const CustomNavbarItemComponents = {
  iconLink: () => NavbarIconLink,
  separator: () => NavbarSeparator,
  cta: () => NavbarCta,
} as const;

export default function NavbarItem({type, ...props}) {
  if (Object.keys(CustomNavbarItemComponents).includes(type)) {
    const Component = CustomNavbarItemComponents[type]();
    return <Component {...props} />;
  } else {
    return <OriginalNavbarItem type={type} {...props} />;
  }
}
