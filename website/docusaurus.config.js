// @ts-check
// Note: type annotations allow type checking and IDEs autocompletion
// https://ricard.dev/how-to-set-docs-as-homepage-for-docusaurus
// https://docusaurus.io/docs/api/themes/configuration#theme
// https://docusaurus.io/docs/markdown-features/code-blocks#line-highlighting
// https://github.com/FormidableLabs/prism-react-renderer/tree/master/packages/prism-react-renderer/src/themes

const path = require('path');

const lightCodeTheme = require('prism-react-renderer').themes.vsDark;
const darkCodeTheme = require('prism-react-renderer').themes.nightOwl;
const latestReleasePlugin = require('./plugins/fetch-latest-release');

const BASE_URL = '';

/** @type {import('@docusaurus/types').Config} */
const config = {
    title: 'atmos',
    tagline: 'Universal tool for DevOps and Cloud Automation',
    url: 'https://atmos.tools',
    baseUrl: `${BASE_URL}/`,
    onBrokenLinks: 'throw',
    onBrokenMarkdownLinks: 'warn',
    favicon: 'img/atmos-logo.png',

    // GitHub pages deployment config.
    // If you aren't using GitHub pages, you don't need these.
    organizationName: 'cloudposse',
    projectName: 'atmos',

    // Even if you don't use internalization, you can use this field to set useful
    // metadata like html lang.
    i18n: {
        defaultLocale: 'en',
        locales: ['en'],
    },

    scripts: [
    ],

    plugins: [
        [
            'docusaurus-plugin-image-zoom', {},
        ],
        [
            '@docusaurus/plugin-client-redirects', {
                redirects: [

                    {
                        from: '/reference/terraform-limitations',
                        to: '/introduction/why-atmos'
                    }
                ],
            },
        ],
        [
            '@grnet/docusaurus-terminology', {
                docsDir: './docs/',
                termsDir: './reference/glossary/',
                glossaryFilepath: './docs/reference/glossary/index.mdx',
                glossaryComponentPath: '../../../src/components/glossary/Glossary.tsx'
        }],
        [
            'custom-loaders', {}
        ],
        [
            '@docusaurus/plugin-google-tag-manager',
            {
                containerId: 'GTM-KQ62MGX9',
            },
        ],
        [
            "posthog-docusaurus",
            {
              apiKey: "phc_G3idXOACKt4vIzgRu2FVP8ORO1D2VlkeEwX9mE2jDvT",
              appUrl: "https://us.i.posthog.com",
              enableInDevelopment: false, // optional
            },
        ],
        [
            'docusaurus-plugin-sentry',
            {
              DSN: 'b022344b0e7cc96f803033fff3b377ee@o56155.ingest.us.sentry.io/4507472203087872',
            },
        ],
        [
            path.resolve(__dirname, 'plugins', 'fetch-latest-release'), {}
        ]
    ],

    presets: [
        [
            'classic',
            /** @type {import('@docusaurus/preset-classic').Options} */
            ({
                docs: {
                    routeBasePath: '/',
                    sidebarPath: require.resolve('./sidebars.js'),
                    editUrl: ({versionDocsDirPath, docPath, locale}) => {
                        return `https://github.com/cloudposse/atmos/edit/master/website/${versionDocsDirPath}/${docPath}`;
                    },
                    exclude: ['README.md'],
                },
                blog: {
                    showReadingTime: true,
                    editUrl: ({versionDocsDirPath, docPath, locale}) => {
                        return `https://github.com/cloudposse/atmos/edit/master/website/${versionDocsDirPath}/${docPath}`;
                    },
                    exclude: ['README.md'],
                },
                theme: {
                    customCss: require.resolve('./src/css/custom.css'),
                },

            }),
        ],
    ],
    themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
        ({
            docs: {
                sidebar: {
                    hideable: true,
                    autoCollapseCategories: true,
                },
            },
            navbar: {
                title: 'atmos',
                logo: {
                    alt: 'Atmos Logo',
                    src: '/img/atmos-logo.svg',
                    srcDark: '/img/atmos-logo-bw.svg',
                    href: '/',
                    target: '_self',
                    height: 36
                },
                items: [
                    {
                        label: `Latest Release`,
                        href: `https://github.com/cloudposse/atmos/releases/latest`,
                        position: 'left',
                        className: 'latest-release-link'  // Add a class to identify this link
                    },
                    {
                        type: 'doc',
                        docId: 'introduction/index',
                        position: 'left',
                        label: 'Learn',
                    },
                    {
                        to: '/cli',
                        position: 'left',
                        label: 'Reference'
                    },
                    {
                        label: 'Community',
                        position: 'left',
                        to: '/community'
                    },
                    // Algolia search configuration
                    {
                        type: 'search',
                        position: 'right',
                    },
                    {
                        href: 'https://github.com/cloudposse/atmos',
                        position: 'right',
                        className: 'header-github-link',
                        'aria-label': 'GitHub repository',
                    },
                    {
                        to: 'https://cloudposse.com/services/',
                        label: 'Get Help',
                        position: 'right',
                        className: 'button button--primary navbar-cta-button'
                    }
                ],
            },
            prism: {
                theme: lightCodeTheme,
                darkTheme: darkCodeTheme,
                // https://prismjs.com/#supported-languages
                additionalLanguages: ['hcl', 'bash']
            },
            algolia: {
                appId: process.env.ALGOLIA_APP_ID || '32YOERUX83',
                apiKey: process.env.ALGOLIA_SEARCH_API_KEY || '557985309adf0e4df9dcf3cb29c61928', // this is SEARCH ONLY API key and is not sensitive information
                indexName: process.env.ALGOLIA_INDEX_NAME || 'atmos.tools',
                contextualSearch: false
            },
            zoom: {
                selector: '.markdown :not(em) > img',
                config: {
                    // options you can specify via https://github.com/francoischalifour/medium-zoom#usage
                    background: {
                        light: 'rgb(255, 255, 255)',
                        dark: 'rgb(50, 50, 50)'
                    }
                }
            },
            announcementBar: {
                id: 'refarch-announcement',
                content:
                  'Try Cloud Posse\'s <a href="https://docs.cloudposse.com">Reference Architecture for AWS, Datadog & GitHub Actions</a> using Atmos',
                backgroundColor: 'var(--announcement-bar-background)',
                textColor: 'var(--announcement-bar-text-color)',
                isCloseable: true,
            },
            colorMode: {
                // "light" | "dark"
                defaultMode: 'dark',

                // Hides the switch in the navbar
                // Useful if you want to force a specific mode
                disableSwitch: false,

                // Should respect the user's color scheme preference
                // "light" | "dark" | "system"
                respectPrefersColorScheme: false,
              },

              mermaid: {
                theme: {
                    light: 'neutral',
                    dark: 'dark',

                },
              },
        }),

    customFields: {
        latestRelease: 'v0.0.0', // initial placeholder
        },

    markdown: {
        mermaid: true,
    },

    themes: ['@docusaurus/theme-mermaid', 'docusaurus-json-schema-plugin']
};

module.exports = config;
