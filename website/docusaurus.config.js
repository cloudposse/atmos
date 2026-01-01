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
const rehypeDtIds = require('./plugins/rehype-dt-ids');

const BASE_URL = '';

/** @type {import('@docusaurus/types').Config} */
const config = {
    title: 'atmos',
    tagline: 'Universal tool for DevOps and Cloud Automation',
    url: 'https://atmos.tools',
    baseUrl: `${BASE_URL}/`,
    onBrokenLinks: 'throw',
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
                        from: '/blog',
                        to: '/changelog'
                    },
                    {
                        from: '/introduction',
                        to: '/intro'
                    },
                    // Redirects for introduction subpages
                    {from: '/introduction/faq', to: '/faq'},
                    {from: '/introduction/features', to: '/features'},
                    {from: '/introduction/use-cases', to: '/use-cases'},
                    {from: '/introduction/index', to: '/intro'},
                    {from: '/introduction/why-atmos/why-atmos', to: '/intro/why-atmos'},
                    // Redirects for integrations pages moved to cli/configuration
                    {from: '/integrations/atlantis', to: '/cli/configuration/integrations/atlantis'},
                    {from: '/integrations/integrations', to: '/cli/configuration/integrations'},
                    {
                        from: '/reference/terraform-limitations',
                        to: '/intro/why-atmos'
                    },
                    // Backend documentation reorganization
                    {
                        from: '/core-concepts/components/terraform/state-backend',
                        to: '/components/terraform/remote-state'
                    },
                    {
                        from: '/core-concepts/components/terraform/remote-state',
                        to: '/components/terraform/remote-state'
                    },
                    // Component Catalog redirects for reorganization
                    {
                        from: '/design-patterns/component-catalog-with-mixins',
                        to: '/design-patterns/component-catalog/with-mixins'
                    },
                    {
                        from: '/design-patterns/component-catalog-template',
                        to: '/design-patterns/component-catalog/template'
                    },
                    // Redirects for template functions moved to /functions/template/
                    {
                        from: '/core-concepts/stacks/templates/functions/atmos.Component',
                        to: '/functions/template/atmos.Component'
                    },
                    {
                        from: '/core-concepts/stacks/templates/functions/atmos.GomplateDatasource',
                        to: '/functions/template/atmos.GomplateDatasource'
                    },
                    {
                        from: '/core-concepts/stacks/templates/functions/atmos.Store',
                        to: '/functions/template/atmos.Store'
                    },
                    // Redirects for YAML functions moved to /functions/yaml/
                    {
                        from: '/core-concepts/stacks/yaml-functions/env',
                        to: '/functions/yaml/env'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/exec',
                        to: '/functions/yaml/exec'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/include',
                        to: '/functions/yaml/include'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/include.raw',
                        to: '/functions/yaml/include.raw'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/repo-root',
                        to: '/functions/yaml/repo-root'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/store.get',
                        to: '/functions/yaml/store.get'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/store',
                        to: '/functions/yaml/store'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/template',
                        to: '/functions/yaml/template'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/terraform.output',
                        to: '/functions/yaml/terraform.output'
                    },
                    {
                        from: '/core-concepts/stacks/yaml-functions/terraform.state',
                        to: '/functions/yaml/terraform.state'
                    },
                    // Redirect for the YAML functions index page
                    {
                        from: '/core-concepts/stacks/yaml-functions',
                        to: '/functions'
                    },
                    // Redirect for the main functions index page
                    {
                        from: '/core-concepts/stacks/templates/functions',
                        to: '/functions'
                    },
                    // Alternative paths that might have been used
                    {
                        from: '/core-concepts/template-functions',
                        to: '/functions'
                    },
                    // Redirects for reorganized stack configuration pages
                    {
                        from: '/core-concepts/stacks/imports',
                        to: '/stacks/imports'
                    },
                    {
                        from: '/core-concepts/stacks/inheritance/inheritance',
                        to: '/howto/inheritance'
                    },
                    {
                        from: '/core-concepts/stacks/inheritance',
                        to: '/howto/inheritance'
                    },
                    {
                        from: '/stacks/inheritance',
                        to: '/howto/inheritance'
                    },
                    {
                        from: '/core-concepts/stacks/inheritance/mixins',
                        to: '/howto/mixins'
                    },
                    {
                        from: '/stacks/mixins',
                        to: '/howto/mixins'
                    },
                    {
                        from: '/core-concepts/stacks/overrides',
                        to: '/stacks/overrides'
                    },
                    {
                        from: '/core-concepts/stacks/dependencies',
                        to: '/stacks/settings/depends_on'
                    },
                    {
                        from: '/core-concepts/stacks/hooks',
                        to: '/stacks/hooks'
                    },
                    {
                        from: '/core-concepts/stacks/catalogs',
                        to: '/howto/catalogs'
                    },
                    {
                        from: '/stacks/catalogs',
                        to: '/howto/catalogs'
                    },
                    // Redirects for workflow pages moved to top level
//                     {
//                         from: '/core-concepts/workflows',
//                         to: '/workflows/workflows'
//                     },
//                     {
//                         from: '/core-concepts/workflows/workflows',
//                         to: '/workflows/workflows'
//                     },
                    // Redirects for vendoring pages moved to top level
                    {
                        from: '/core-concepts/vendor',
                        to: '/vendor/'
                    },
                    {
                        from: '/core-concepts/vendor/vendor',
                        to: '/vendor/'
                    },
                    {
                        from: '/vendoring/vendor',
                        to: '/vendor/'
                    },
                    {
                        from: '/vendoring',
                        to: '/vendor/'
                    },
                    {
                        from: '/core-concepts/vendor/vendor-package',
                        to: '/vendor/component-manifest/'
                    },
                    {
                        from: '/vendoring/component-manifest',
                        to: '/vendor/component-manifest/'
                    },
                    {
                        from: '/core-concepts/vendor/vendor-lock',
                        to: '/vendor/vendor-config'
                    },
                    {
                        from: '/vendoring/vendor-manifest',
                        to: '/vendor/vendor-config'
                    },
                    {
                        from: '/vendor/config/vendor-config',
                        to: '/vendor/vendor-config'
                    },
                    // Redirects for validation pages moved to top level
                    {
                        from: '/core-concepts/validate',
                        to: '/validation/validating'
                    },
                    {
                        from: '/core-concepts/validate/validate',
                        to: '/validation/validating'
                    },
                    {
                        from: '/core-concepts/validate/json-schema',
                        to: '/validation/json-schema'
                    },
                    {
                        from: '/core-concepts/validate/opa',
                        to: '/validation/opa'
                    },
                    {
                        from: '/core-concepts/validate/editorconfig',
                        to: '/validation/editorconfig-validation'
                    },
                    {
                        from: '/core-concepts/validate/terraform-variables',
                        to: '/validation/terraform-variables'
                    },
                    // Redirects for template pages moved to top level
//                     {
//                         from: '/core-concepts/stacks/templates',
//                         to: '/templates/templates'
//                     },
                    {
                        from: '/core-concepts/stacks/templates/datasources',
                        to: '/templates/datasources'
                    },
                    // Redirects for directory renames: core-concepts → learn
                    {from: '/core-concepts/why-atmos', to: '/learn/why-atmos'},
                    {from: '/core-concepts/concepts-overview', to: '/learn/concepts-overview'},
                    {from: '/core-concepts/first-stack', to: '/learn/first-stack'},
                    {from: '/core-concepts/yaml-guide', to: '/learn/yaml'},
                    {from: '/learn/yaml-guide', to: '/learn/yaml'},
                    {from: '/core-concepts/imports-basics', to: '/learn/imports-basics'},
                    {from: '/core-concepts/inheritance-basics', to: '/learn/inheritance-basics'},
                    {from: '/core-concepts/organizing-stacks', to: '/learn/organizing-stacks'},
                    {from: '/core-concepts/connecting-components', to: '/learn/connecting-components'},
                    {from: '/core-concepts/next-steps', to: '/learn/next-steps'},
                    // Terraform source command reorganization
                    {from: '/cli/commands/terraform/terraform-source', to: '/cli/commands/terraform/source'},
                    {from: '/cli/commands/terraform/terraform-source-pull', to: '/cli/commands/terraform/source/pull'},
                    {from: '/cli/commands/terraform/terraform-source-list', to: '/cli/commands/terraform/source/list'},
                    {from: '/cli/commands/terraform/terraform-source-describe', to: '/cli/commands/terraform/source/describe'},
                    {from: '/cli/commands/terraform/terraform-source-delete', to: '/cli/commands/terraform/source/delete'},
                    // Terraform generate command reorganization
                    {from: '/cli/commands/terraform/terraform-generate-backend', to: '/cli/commands/terraform/generate/backend'},
                    {from: '/cli/commands/terraform/terraform-generate-backends', to: '/cli/commands/terraform/generate/backends'},
                    {from: '/cli/commands/terraform/terraform-generate-planfile', to: '/cli/commands/terraform/generate/planfile'},
                    {from: '/cli/commands/terraform/terraform-generate-varfile', to: '/cli/commands/terraform/generate/varfile'},
                    {from: '/cli/commands/terraform/terraform-generate-varfiles', to: '/cli/commands/terraform/generate/varfiles'},
                    // Legacy generate command paths (without terraform- prefix)
                    {from: '/cli/commands/terraform/generate-backend', to: '/cli/commands/terraform/generate/backend'},
                    {from: '/cli/commands/terraform/generate-backends', to: '/cli/commands/terraform/generate/backends'},
                    {from: '/cli/commands/terraform/generate-planfile', to: '/cli/commands/terraform/generate/planfile'},
                    {from: '/cli/commands/terraform/generate-varfile', to: '/cli/commands/terraform/generate/varfile'},
                    {from: '/cli/commands/terraform/generate-varfiles', to: '/cli/commands/terraform/generate/varfiles'},
                ],
            },
        ],
        [
            path.resolve(__dirname, './plugins/glossary-tooltips'), {
                docsDir: './docs/',
                termsDir: './docs/glossary/',
                glossaryFilepath: './docs/glossary/index.mdx',
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
              apiKey: "phc_uoINtjtkrInRNNYGdTU6VyFzEL2fWwB8le1xSxvSOjk",
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
        ],
        [
            path.resolve(__dirname, 'plugins', 'fetch-github-stars'), {}
        ],
        [
            path.resolve(__dirname, 'plugins', 'blog-release-data'), {}
        ],
        [
            path.resolve(__dirname, 'plugins', 'docusaurus-plugin-llms-txt'),
            {
                generateLlmsTxt: true,
                generateLlmsFullTxt: true,
                llmsTxtFilename: 'llms.txt',
                llmsFullTxtFilename: 'llms-full.txt',
                docsDir: 'docs',
                includeBlog: true,
                includeOrder: [
                    'intro/*',
                    'quick-start/*',
                    'install/*',
                    'learn/*',
                    'cli/*',
                ],
            },
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
                        return `https://github.com/cloudposse/atmos/edit/main/website/${versionDocsDirPath}/${docPath}`;
                    },
                    exclude: ['README.md', '**/_*/**', '**/_*'],
                    rehypePlugins: [rehypeDtIds],
                },
                blog: {
                    routeBasePath: 'changelog',
                    showReadingTime: true,
                    postsPerPage: 'ALL',
                    blogSidebarCount: 'ALL',
                    blogSidebarTitle: 'All posts',
                    blogTitle: 'Atmos Changelog',
                    blogDescription: 'Release notes for Atmos',
                    include: ['**/*.{md,mdx}'],
                    editUrl: ({versionDocsDirPath, docPath, locale}) => {
                        return `https://github.com/cloudposse/atmos/edit/main/website/${versionDocsDirPath}/${docPath}`;
                    },
                    exclude: ['README.md'],
                    blogSidebarTitle: 'Recent Changes',
                    blogSidebarCount: 'ALL',
                    showReadingTime: true,
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
                        docId: 'intro/index',
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
                    {
                        label: 'Changelog',
                        position: 'left',
                        to: '/changelog'
                    },
                    {
                        label: 'Roadmap',
                        position: 'left',
                        to: '/roadmap'
                    },
                    // GitHub stars badge
                    {
                        type: 'custom-github-stars',
                        position: 'right',
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
                        to: 'https://cloudposse.com/services/support/',
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
                contextualSearch: false,
                // DocSearch v4 Ask AI integration
                // https://docsearch.algolia.com/docs/v4/askai/
                askAi: {
                    assistantId: process.env.ALGOLIA_ASKAI_ASSISTANT_ID || '0ad3822f-e071-402e-bc54-b2d89f3c32d1',
                }
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
        hooks: {
            onBrokenMarkdownLinks: 'warn',
        },
    },

    themes: ['@docusaurus/theme-mermaid']
};

module.exports = config;
