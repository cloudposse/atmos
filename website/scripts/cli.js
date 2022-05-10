const fs = require('fs');
const utils = require('./utils');
const cliJSON = require('./data/cli.json');

const commandToKebab = (str) =>
  str
    .replace('atmos ', '')
    .replace(/([a-z])([A-Z])/g, '$1-$2')
    .replace(/[\s_]+/g, '-')
    .toLowerCase();

(async function () {
  // console.log(cliJSON);
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

${description}`;
  } else {
    return `
${summary}

\`\`\`shell
$ ${name} [options]
\`\`\`

${description}`;
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

${utils.renderReference(inputs, {
    Head: (input) => input.name,
    Description: (input) => utils.renderMarkdown(input.summary),
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

${utils.renderReference(options, {
    Head: (option) => utils.renderOptionSpec(option),
    Description: (option) => utils.renderMarkdown(option.summary),
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

${utils.renderReference(options, {
    Head: (option) => utils.renderOptionSpec(option),
    Description: (option) => `<div>${utils.renderMarkdown(option.summary)}</div>`,
    Aliases: (option) =>
      option.aliases.length > 0 ? option.aliases.map((alias) => `<code>-${alias}</code>`).join(' ') : null,
    Default: (option) => (option.default && option.type === 'string' ? option.default : null),
  })}

`;
}
