/**
 * Custom site-wide footer (swizzled, ejected from `@theme/Footer`).
 *
 * The stock Docusaurus footer renders `null` when `themeConfig.footer` is not
 * configured. We intentionally do not configure `themeConfig.footer`; instead
 * this component owns the footer's markup and styling directly, so it renders
 * a rich sitemap "directory" of links on every page.
 */

import React, { type ReactElement } from 'react';
import Link from '@docusaurus/Link';
import { RiGithubFill } from 'react-icons/ri';
import { SiSlack } from 'react-icons/si';

import { footerColumns, socialLinks, type FooterLink } from './links';
import styles from './styles.module.css';

const socialIcons: Record<string, ReactElement> = {
  github: <RiGithubFill aria-hidden="true" />,
  slack: <SiSlack aria-hidden="true" />,
};

function FooterItemLink({ item }: { item: FooterLink }): ReactElement {
  // Internal routes use Docusaurus `<Link>`; external links use a plain anchor
  // that opens in a new tab with safe rel attributes.
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
    </Link>
  );
}

export default function Footer(): ReactElement {
  const year = new Date().getFullYear();

  return (
    <footer className={styles.footer}>
      <div className={styles.container}>
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

        <div className={styles.bottomBar}>
          <div className={styles.brand}>
            <Link to="/" className={styles.brandLink}>
              <img
                className={styles.brandLogo}
                src="/img/atmos-logo-bw.svg"
                alt="Atmos"
                height={28}
              />
            </Link>
            <span className={styles.copyright}>
              &copy; {year} Cloud Posse, LLC
            </span>
          </div>

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
      </div>
    </footer>
  );
}
