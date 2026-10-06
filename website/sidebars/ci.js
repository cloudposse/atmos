// Workflow guides live with CI/CD; configuration links retain their canonical pages.
module.exports = {
  type: 'category',
  label: 'CI/CD',
  description: 'Plan, review, and deploy infrastructure in your CI workflows.',
  className: 'sidebar-title',
  link: {type: 'doc', id: 'ci/ci'},
  items: [
    {
      type: 'category',
      label: 'GitHub Actions',
      collapsed: false,
      link: {type: 'doc', id: 'integrations/github-actions/index'},
      items: [
        {type: 'doc', id: 'integrations/github-actions/setup-atmos', label: 'Setup Atmos'},
        {type: 'doc', id: 'integrations/github-actions/plan-on-pull-request', label: 'Plan on Pull Request'},
        {type: 'doc', id: 'integrations/github-actions/apply-on-merge', label: 'Apply on Merge'},
        {type: 'doc', id: 'integrations/github-actions/deploy-affected', label: 'Deploy Affected Components'},
        {type: 'doc', id: 'integrations/github-actions/deploy-all', label: 'Deploy All Components'},
        {type: 'doc', id: 'integrations/github-actions/deployment-approvals', label: 'Deployment Approvals'},
        {type: 'doc', id: 'integrations/github-actions/authentication', label: 'Authentication & Permissions'},
        {type: 'doc', id: 'integrations/github-actions/validate-workflows', label: 'Validate Workflows'},
      ],
    },
    {type: 'link', customProps: {navigationReference: true}, href: '/cli/configuration/ci', label: 'CI Configuration'},
    {type: 'doc', id: 'ci/planfile-storage', label: 'Planfile Storage'},
    {type: 'link', customProps: {navigationReference: true}, href: '/cli/configuration/ci/cache', label: 'Build Cache'},
    {type: 'link', customProps: {navigationReference: true}, href: '/cli/configuration/ci/output', label: 'Outputs'},
    {type: 'doc', id: 'ci/job-summaries', label: 'Job Summaries'},
    {type: 'link', customProps: {navigationReference: true}, href: '/cli/configuration/ci/comments', label: 'Pull Request Comments'},
    {type: 'link', customProps: {navigationReference: true}, href: '/cli/configuration/ci/checks', label: 'Status Checks'},
    {type: 'doc', id: 'ci/migrate-legacy-actions', label: 'Migrate from Legacy Actions'},
  ],
};
