// @ts-check
// Note: type annotations allow type checking and IDEs autocompletion
// https://ricard.dev/how-to-set-docs-as-homepage-for-docusaurus/

const lightCodeTheme = require('prism-react-renderer/themes/github');
const darkCodeTheme = require('prism-react-renderer/themes/dracula');

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
    organizationName: 'CloudPosse',
    projectName: 'atmos',

    // Even if you don't use internalization, you can use this field to set useful
    // metadata like html lang. For example, if your site is Chinese, you may want
    // to replace "en" with "zh-Hans".
    i18n: {
        defaultLocale: 'en',
        locales: ['en'],
    },

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
            navbar: {
                title: 'atmos',
                logo: {
                    alt: 'atmos logo',
                    src: '/img/atmos-logo.svg',
                    srcDark: '/img/atmos-logo.svg',
                    href: '/',
                    target: '_self',
                    height: 36
                },
                items: [
                    {
                        type: 'doc',
                        docId: 'introduction',
                        position: 'left',
                        label: 'Docs',
                    },
                    {
                        to: '/cli/configuration',
                        position: 'left',
                        label: 'CLI'
                    },
                    {
                        to: '/blog',
                        label: 'Blog',
                        position: 'left'
                    },
                    {
                        href: 'https://github.com/cloudposse/atmos',
                        label: 'GitHub',
                        position: 'right',
                    },
                ],
            },
            prism: {
                theme: lightCodeTheme,
                darkTheme: darkCodeTheme,
            },
        }),
};

module.exports = config;
