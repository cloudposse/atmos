/**
 * Custom site-wide footer (swizzled, ejected from `@theme/Footer`).
 *
 * The stock Docusaurus footer renders `null` when `themeConfig.footer` is not
 * configured. We intentionally do not configure `themeConfig.footer`; instead
 * this component owns the footer's markup and styling directly, so it renders
 * a rich sitemap "directory" of links on every page.
 *
 * The layout mirrors the Atmos Pro footer: a brand block (wordmark + tagline +
 * inline social icons) on the left, the link columns to the right, and a
 * divider'd bottom bar with the copyright and a Cloud Posse attribution.
 */

import React, { type ReactElement } from 'react';
import Link from '@docusaurus/Link';
import {
  RiExternalLinkLine,
  RiGithubFill,
  RiLinkedinBoxFill,
  RiSlackFill,
  RiTwitterXFill,
  RiYoutubeFill,
} from 'react-icons/ri';

import {
  brandTagline,
  footerColumns,
  socialLinks,
  type FooterLink,
} from './links';
import styles from './styles.module.css';

const socialIcons: Record<string, ReactElement> = {
  github: <RiGithubFill aria-hidden="true" />,
  twitter: <RiTwitterXFill aria-hidden="true" />,
  linkedin: <RiLinkedinBoxFill aria-hidden="true" />,
  youtube: <RiYoutubeFill aria-hidden="true" />,
  slack: <RiSlackFill aria-hidden="true" />,
};

function FooterItemLink({ item }: { item: FooterLink }): ReactElement {
  // Internal routes use Docusaurus `<Link>`; external links use a plain anchor
  // that opens in a new tab with safe rel attributes and an external-link glyph.
  if (item.to) {
    return (
      <Link className={styles.link} to={item.to}>
        {item.label}
      </Link>
    );
  }

  return (
    <Link
      className={styles.link}
      href={item.href}
      target="_blank"
      rel="noopener noreferrer"
    >
      {item.label}
      <RiExternalLinkLine className={styles.externalIcon} aria-hidden="true" />
    </Link>
  );
}

export default function Footer(): ReactElement {
  const year = new Date().getFullYear();

  return (
    <footer className={styles.footer}>
      <div className={styles.container}>
        <div className={styles.top}>
          <div className={styles.brand}>
            <Link to="/media-kit" className={styles.brandLink} aria-label="Atmos brand kit">
              <img
                className={`${styles.brandLogo} ${styles.logoLight}`}
                src="/img/atmos-logo-gradient-on-light.svg"
                alt="Atmos"
                height={32}
              />
              <img
                className={`${styles.brandLogo} ${styles.logoDark}`}
                src="/img/atmos-logo-gradient.svg"
                alt="Atmos"
                height={32}
              />
            </Link>
            <p className={styles.tagline}>{brandTagline}</p>
            <div className={styles.social}>
              {socialLinks.map((social) => (
                <a
                  key={social.label}
                  className={styles.socialLink}
                  href={social.href}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label={social.label}
                  title={social.label}
                >
                  {socialIcons[social.icon]}
                </a>
              ))}
            </div>
          </div>

          <div className={styles.columns}>
            {footerColumns.map((column) => (
              <nav
                key={column.title}
                className={styles.column}
                aria-label={column.title}
              >
                <h2 className={styles.columnTitle}>{column.title}</h2>
                <ul className={styles.itemList}>
                  {column.items.map((item) => (
                    <li key={item.label} className={styles.item}>
                      <FooterItemLink item={item} />
                    </li>
                  ))}
                </ul>
              </nav>
            ))}
          </div>
        </div>

        <div className={styles.bottomBar}>
          <span className={styles.copyright}>
            &copy; {year} Cloud Posse, LLC. All rights reserved.
          </span>
        </div>
      </div>
    </footer>
  );
}
