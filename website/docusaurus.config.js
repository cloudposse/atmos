const path = require('path');

const BASE_URL = '';

module.exports = {
  title: 'Atmos Documentation',
  tagline: 'atmos is a universal tool for DevOps and Cloud Automation (works with terraform, helm, helmfile, aws, etc.)',
  url: 'https://atmos.tools',
  baseUrl: `${BASE_URL}/`,
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
    localeConfigs: {
      en: {label: 'English'}
    },
  },
  onBrokenLinks: 'warn',
  onBrokenMarkdownLinks: 'warn',
  favicon: 'logos/atmos-logo.png',
  organizationName: 'Cloud Posse LLC',
  projectName: 'atmos-docs',
  themeConfig: {
    colorMode: {
      defaultMode: 'light',
    },
    navbar: {
      hideOnScroll: false,
      logo: {
        alt: 'atmos logo',
        src: '/logos/atmos-docs-logo-dark.svg',
        srcDark: '/logos/atmos-docs-logo-light.svg',
        href: '/',
        target: '_self',
        height: 45
      },
      items: [
        {
          type: 'doc',
          docId: 'index',
          label: 'Guide',
          position: 'left',
        },
        {
          type: 'doc',
          docId: 'cli',
          label: 'CLI',
          position: 'left',
        },
        {
          type: 'search',
          position: 'right',
        },
        {
          label: 'Community',
          position: 'right',
          items: [
            {
              href: 'https://cloudposse.com/community/',
              label: 'Community Hub',
              target: '_blank',
              rel: null,
            },
            {
              href: 'https://podcast.cloudposse.com/',
              label: 'Podcast',
              target: '_blank',
              rel: null,
            },
            {
              href: 'https://cloudposse.com/newsletter/',
              label: 'Newsletter',
              target: '_blank',
              rel: null,
            },
            {
              href: 'https://youtube.com/cloudposse',
              label: 'YouTube',
              target: '_blank',
              rel: null,
            },
            {
              href: 'https://twitter.com/cloudposse/',
              label: 'Twitter',
              target: '_blank',
              rel: null,
            },
            {
              href: 'https://facebook.com/cloudposse/',
              label: 'Facebook',
              target: '_blank',
              rel: null,
            },
            {
              href: 'https://linkedin.com/in/osterman',
              label: 'LinkedIn',
              target: '_blank',
              rel: null,
            },
            {
              href: 'https://cloudposse.com/feed/',
              label: 'Feed',
              target: '_blank',
              rel: null,
            },
          ],
          className: 'navbar__link--community',
        },
        {
          label: 'Support',
          position: 'right',
          items: [
            {
              href: 'https://cloudposse.com/quiz/',
              label: 'Help Center',
              target: '_blank',
              rel: null,
            },
            {
              href: 'https://cloudposse.com/faq/',
              label: 'FAQ',
              target: '_blank',
              rel: null,
            },
          ],
          className: 'navbar__link--support',
        },
        {
          type: 'separator',
          position: 'right',
        },
        {
          type: 'iconLink',
          position: 'right',
          icon: {
            alt: 'github logo',
            src: `/logos/github.svg`,
            href: 'https://github.com/cloudposse/atmos',
            target: '_blank',
          },
        },
        {
          type: 'iconLink',
          position: 'right',
          icon: {
            alt: 'twitter logo',
            src: `/logos/twitter.svg`,
            href: 'https://twitter.com/cloudposse/',
            target: '_blank',
          },
        },
      ],
    },
    tagManager: {
      trackingID: 'GTM-WQWH2XV',
    },
    prism: {
      theme: {plain: {}, styles: []},
      // https://github.com/FormidableLabs/prism-react-renderer/blob/master/src/vendor/prism/includeLangs.js
      additionalLanguages: ['shell-session', 'http'],
    },
    algolia: {
      appId: 'UZ3HBXELUD',
      apiKey: '1568ce161ef1c20cb929a12dd33df7be',
      indexName: 'atmos_docs',
      contextualSearch: true,
    },
  },
  plugins: [
    'docusaurus-plugin-sass',
    [
      'docusaurus-plugin-module-alias',
      {
        alias: {
          'styled-components': path.resolve(__dirname, './node_modules/styled-components'),
          react: path.resolve(__dirname, './node_modules/react'),
          'react-dom': path.resolve(__dirname, './node_modules/react-dom'),
          '@components': path.resolve(__dirname, './src/components'),
        },
      },
    ],
    [
      '@docusaurus/plugin-content-docs',
      {
        routeBasePath: '/',
        sidebarPath: require.resolve('./sidebars.js'),
        editUrl: ({versionDocsDirPath, docPath, locale}) => {
          return `https://github.com/cloudposse/atmos/edit/master/website/${versionDocsDirPath}/${docPath}`;
        },
        exclude: ['README.md'],
        lastVersion: 'current',
      },
    ],
    '@docusaurus/plugin-content-pages',
    '@docusaurus/plugin-debug',
    '@docusaurus/plugin-sitemap',
    '@ionic-internal/docusaurus-plugin-tag-manager'
  ],
  themes: [
    [
      // overriding the standard docusaurus-theme-classic to provide custom schema
      path.resolve(__dirname, 'docusaurus-theme-classic'),
      {
        customCss: [
          require.resolve('./node_modules/modern-normalize/modern-normalize.css'),
          require.resolve('./node_modules/@ionic-internal/ionic-ds/dist/tokens/tokens.css'),
          require.resolve('./src/styles/custom.scss'),
        ],
      },
    ],
    path.resolve(__dirname, './node_modules/@docusaurus/theme-search-algolia'),
  ],
  customFields: {},
};
