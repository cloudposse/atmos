import fs from 'fs';
import {unified} from 'unified';
import markdown from 'remark-parse';
import html from 'remark-html';
import cliJSON from './cli.json' assert {type: 'json'};

const commandToKebab = (str) =>
    str
        .replace('atmos ', '')
        .replace(/([a-z])([A-Z])/g, '$1-$2')
        .replace(/[\s_]+/g, '-')
        .toLowerCase();

(async function () {
    cliJSON.commands.map(writePage);
})();

function writePage(page) {
    const data = [
        renderFrontmatter(page),
        renderIntro(page),
        renderExamples(page),
        renderInputs(page),
        renderOptions(page),
        renderAdvancedOptions(page),
    ].join('');

    const path = `docs/cli/commands/${commandToKebab(page.name)}.md`;
    fs.writeFileSync(path, data);
}

function renderFrontmatter({name}) {
    const shortName = name.replace('atmos ', '');
    const slug = commandToKebab(shortName);

    const frontmatter = {
        title: name,
        sidebar_label: shortName,
    };

    return `---
${Object.entries(frontmatter)
        .map(([key, value]) => `${key}: ${typeof value === 'string' ? `"${value.replace('"', '\\"')}"` : value}`)
        .join('\n')}
---
`;
}

function renderIntro({description, summary, name, options}) {
    let ops = !!options ? options.filter((option) => !option.groups.includes('advanced')) : [];

    if (ops.length === 0) {
        return `
${summary}

\`\`\`shell
$ ${name}
\`\`\`

${(description + "").replace(/,\n/g, "\n")}`;
    } else {
        return `
${summary}

\`\`\`shell
$ ${name} [options]
\`\`\`

${(description + "").replace(/,\n/g, "\n")}`;
    }
}

function renderExamples({exampleCommands}) {
    if (!exampleCommands || exampleCommands.length === 0) {
        return '';
    }

    return `
## Examples

\`\`\`shell
${exampleCommands.map((command) => `$ ${command}`).join('\n')}
\`\`\`
`;
}

function renderInputs({inputs}) {
    if (inputs.length === 0) {
        return '';
    }

    return `
## Inputs

${renderReference(inputs, {
        Head: (input) => input.name,
        Description: (input) => renderMarkdown(input.summary),
    })}

`;
}

function renderOptions({options}) {
    options = !!options ? options.filter((option) => !option.groups.includes('advanced')) : [];

    if (options.length === 0) {
        return '';
    }

    return `
## Options

${renderReference(options, {
        Head: (option) => renderOptionSpec(option),
        Description: (option) => renderMarkdown(option.summary),
        Aliases: (option) =>
            option.aliases.length > 0 ? option.aliases.map((alias) => `<code>-${alias}</code>`).join(' ') : null,
        Default: (option) => (option.default && option.type === 'string' ? option.default : null),
    })}
`;
}

function renderAdvancedOptions({options}) {
    options = options.filter((option) => option.groups.includes('advanced'));

    if (options.length === 0) {
        return '';
    }

    return `
## Advanced Options

${renderReference(options, {
        Head: (option) => renderOptionSpec(option),
        Description: (option) => `<div>${renderMarkdown(option.summary)}</div>`,
        Aliases: (option) =>
            option.aliases.length > 0 ? option.aliases.map((alias) => `<code>-${alias}</code>`).join(' ') : null,
        Default: (option) => (option.default && option.type === 'string' ? option.default : null),
    })}
`;
}

function renderMarkdown(markdownString) {
    return unified().use(markdown).use(html).processSync(markdownString);
}

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
<title></title>
</head>`;
}
