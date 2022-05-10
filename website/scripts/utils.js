const unified = require('unified');
const markdown = require('remark-parse');
const html = require('remark-html');

function renderMarkdown(markdownString) {
  return unified().use(markdown).use(html).processSync(markdownString);
}

// a String equivalent to this component
function renderReference(data, methodKeys) {
  return `
<table className="reference-table">
  ${data
    .map((item) => {
      const {Head, ...keys} = methodKeys;

      return `
      <thead>
        <tr>
          <th colSpan="2">
            <h3>${Head(item)}</h3>
          </th>
        </tr>
      </thead>
      <tbody>
        ${Object.keys(keys)
        .map((name) => {
          const content = keys[name](item);
          if (content) {
            return `
              <tr>
                <th>${name}</th>
                <td>${content}</td>
              </tr>
            `;
          }
        })
        .join(' ')}
      </tbody>`;
    })
    .join('')}
</table>
`;
}

// a String equivalent to this functional component
function renderOptionSpec(option) {
  return `
<a href="#option-${option.name}" id="option-${option.name}">
  --${option.type === 'boolean' && option.default === true ? `no-${option.name}` : option.name}
  ${option.type === 'string' ? `<span class="option-spec"> =&lt;${option.spec.value}&gt;</span>` : ''}
</a>`.replace('\n', '');
}

function gitBranchSVG() {
  return `<svg viewBox="0 0 512 512"><path d="M416 160c0-35.3-28.7-64-64-64s-64 28.7-64 64c0 23.7 12.9 44.3 32 55.4v8.6c0 19.9-7.8 33.7-25.3 44.9-15.4 9.8-38.1 17.1-67.5 21.5-14 2.1-25.7 6-35.2 10.7V151.4c19.1-11.1 32-31.7 32-55.4 0-35.3-28.7-64-64-64S96 60.7 96 96c0 23.7 12.9 44.3 32 55.4v209.2c-19.1 11.1-32 31.7-32 55.4 0 35.3 28.7 64 64 64s64-28.7 64-64c0-16.6-6.3-31.7-16.7-43.1 1.9-4.9 9.7-16.3 29.4-19.3 38.8-5.8 68.9-15.9 92.3-30.8 36-22.8 55-57 55-98.8v-8.6c19.1-11.1 32-31.7 32-55.4zM160 56c22.1 0 40 17.9 40 40s-17.9 40-40 40-40-17.9-40-40 17.9-40 40-40zm0 400c-22.1 0-40-17.9-40-40s17.9-40 40-40 40 17.9 40 40-17.9 40-40 40zm192-256c-22.1 0-40-17.9-40-40s17.9-40 40-40 40 17.9 40 40-17.9 40-40 40z"></path></svg>`;
}

function getHeadTag({title: metaTitle, description: metaDescription} = {}) {
  if (!metaTitle && !metaDescription) return '';

  return `<head>
  ${metaTitle ? `<title>${metaTitle}</title>` : ''}
  ${metaDescription ? `<meta name="description" content="${metaDescription}" />` : ''}
</head>`;
}

module.exports = {
  gitBranchSVG,
  renderOptionSpec,
  renderMarkdown,
  renderReference,
  getHeadTag,
};
