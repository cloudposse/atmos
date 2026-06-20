/**
 * Footer link data for the custom site-wide footer.
 *
 * Keeping the sitemap "directory" of links here keeps `index.tsx` focused on
 * rendering. Internal routes use Docusaurus-relative paths (e.g. `/cli`);
 * external links use absolute URLs and open in a new tab.
 */

export interface FooterLink {
  label: string;
  to?: string; // Internal Docusaurus route.
  href?: string; // External URL.
}

export interface FooterColumn {
  title: string;
  items: FooterLink[];
}

export const footerColumns: FooterColumn[] = [
  {
    title: 'Product',
    items: [
      { label: 'Install', to: '/install' },
      { label: 'Get Started', to: '/intro' },
      { label: 'CLI Reference', to: '/cli' },
      { label: 'Examples', to: '/examples' },
      { label: 'Atmos Pro', to: '/pro' },
    ],
  },
  {
    title: 'Learn',
    items: [
      { label: 'Components', to: '/components' },
      { label: 'Stacks', to: '/stacks' },
      { label: 'Workflows', to: '/workflows' },
      { label: 'Vendoring', to: '/vendor' },
      { label: 'Templates', to: '/templates' },
    ],
  },
  {
    title: 'Resources',
    items: [
      { label: 'Changelog', to: '/changelog' },
      { label: 'Roadmap', to: '/roadmap' },
      {
        label: 'Latest Release',
        href: 'https://github.com/cloudposse/atmos/releases/latest',
      },
      { label: 'Media Kit', to: '/media-kit' },
    ],
  },
  {
    title: 'Community',
    items: [
      { label: 'Community Hub', to: '/community' },
      { label: 'Slack', href: 'https://cloudposse.com/slack' },
      { label: 'Office Hours', href: 'https://cloudposse.com/office-hours' },
      { label: 'Newsletter', href: 'https://newsletter.cloudposse.com' },
      {
        label: 'GitHub Issues',
        href: 'https://github.com/cloudposse/atmos/issues',
      },
    ],
  },
  {
    title: 'Company',
    items: [
      { label: 'Cloud Posse', href: 'https://cloudposse.com' },
      { label: 'Get Help', href: 'https://cloudposse.com/services/support/' },
      { label: 'GitHub', href: 'https://github.com/cloudposse/atmos' },
    ],
  },
];

export interface SocialLink {
  label: string;
  href: string;
  /** Icon key resolved in `index.tsx`. */
  icon: 'github' | 'slack';
}

export const socialLinks: SocialLink[] = [
  { label: 'GitHub', href: 'https://github.com/cloudposse/atmos', icon: 'github' },
  { label: 'Slack', href: 'https://cloudposse.com/slack', icon: 'slack' },
];
