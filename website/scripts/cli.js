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

function renderIntro({description, summary, name}) {
  return `
${summary}

\`\`\`shell
$ ${name} [options]
\`\`\`

${description}`;
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
  options = options.filter((option) => !option.groups.includes('advanced'));

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

// function renderProperties({ props: properties }) {
//   if (properties.length === 0) {
//     return "";
//   }

//   return `
// ## Properties

// ${properties
//   .map(
//     prop => `
// ### ${prop.name}

// | | |
// | --- | --- |
// | **Description** | ${prop.docs.split("\n").join("<br />")} |
// | **Attribute** | \`${prop.attr}\` |
// | **Type** | \`${prop.type.replace(/\|/g, "\\|")}\` |
// | **Default** | \`${prop.default}\` |

// `
//   )
//   .join("\n")}
// `;
// }

// function renderEvents({ events }) {
//   if (events.length === 0) {
//     return "";
//   }

//   return `
// ## Events

// | Name | Description |
// | --- | --- |
// ${events.map(event => `| \`${event.event}\` | ${event.docs} |`).join("\n")}

// `;
// }

// function renderMethods({ methods }) {
//   if (methods.length === 0) {
//     return "";
//   }

//   return `
// ## Methods

// ${methods
//   .map(
//     method => `
// ### ${method.name}

// | | |
// | --- | --- |
// | **Description** | ${method.docs.split("\n").join("<br />")} |
// | **Signature** | \`${method.signature.replace(/\|/g, "\\|")}\` |
// `
//   )
//   .join("\n")}

// `;
// }

// function renderParts({ parts }) {
//   if (parts.length === 0) {
//     return "";
//   }

//   return `
// ## CSS Shadow Parts

// | Name | Description |
// | --- | --- |
// ${parts.map(prop => `| \`${prop.name}\` | ${prop.docs} |`).join("\n")}

// `;
// }

// function renderCustomProps({ styles: customProps }) {
//   if (customProps.length === 0) {
//     return "";
//   }

//   return `
// ## CSS Custom Properties

// | Name | Description |
// | --- | --- |
// ${customProps.map(prop => `| \`${prop.name}\` | ${prop.docs} |`).join("\n")}

// `;
// }

// function renderSlots({ slots }) {
//   if (slots.length === 0) {
//     return "";
//   }

//   return `
// ## Slots

// | Name | Description |
// | --- | --- |
// ${slots.map(slot => `| \`${slot.name}\` | ${slot.docs} |`).join("\n")}

// `;
// }
