import Prism from 'prismjs';

const loadLanguages = require('prismjs/components/'); // tslint:disable-line

export default (code: string, info?: string) => {
  const [lang] = info !== undefined ? info.split(/\s+/).map(s => s.toLowerCase()) : [null];
  const isShell = lang !== null ? shells.includes(lang) : false;
  const language = isShell ? 'shell' : lang;

  // tslint:disable-next-line
  if (!language || Prism.languages[language] === undefined) {
    return `
<docs-code language=${language}>
  <pre><code>${escape(code)}</code></pre>
</docs-code>
`;
  }

  return `
<docs-code language="${language}">
  <pre><code>${Prism.highlight(code, Prism.languages[language], language)}</code></pre>
</docs-code>
`;
};

loadLanguages([
  'bash',
  'json',
  'tsx',
  'typescript',
  'scss'
]);

// `shell` is an alias of `bash`, so we have to extend `bash`.
Prism.languages.bash['prompt'] = /^\s*[\$#]\s+/gm;

const shells = [
  'shell',
  'bash',
  'sh'
];

const escape = (code: string) =>
  code.replace(/[&<>"']/g, char => escapeMap[char]);

const escapeMap: { [key: string]: string; } = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  '\'': '&#39;'
};
