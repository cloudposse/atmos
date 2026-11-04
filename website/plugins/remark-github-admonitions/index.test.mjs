import assert from 'node:assert/strict';
import test from 'node:test';

import remarkGithubAdmonitions from './index.js';

const text = (value) => ({ type: 'text', value });
const paragraph = (...children) => ({ type: 'paragraph', children });
const blockquote = (...children) => ({ type: 'blockquote', children });
const root = (...children) => ({ type: 'root', children });

function run(tree) {
  remarkGithubAdmonitions()(tree);
  return tree;
}

const TYPE_CASES = [
  ['NOTE', 'note'],
  ['TIP', 'tip'],
  ['IMPORTANT', 'important'],
  ['WARNING', 'warning'],
  ['CAUTION', 'danger'],
];

for (const [marker, expected] of TYPE_CASES) {
  test(`[!${marker}] becomes a ${expected} admonition`, () => {
    const quote = blockquote(paragraph(text(`[!${marker}]\nBody text.`)));
    run(root(quote));

    assert.deepEqual(quote.data, { hName: 'admonition', hProperties: { type: expected } });
    assert.equal(quote.children[0].children[0].value, 'Body text.');
  });
}

test('marker is matched case-insensitively', () => {
  const quote = blockquote(paragraph(text('[!warning]\nBody text.')));
  run(root(quote));

  assert.equal(quote.data.hProperties.type, 'warning');
});

test('marker on its own paragraph is dropped and later content is kept', () => {
  const body = paragraph(text('First paragraph.'));
  const more = paragraph(text('Second paragraph.'));
  const quote = blockquote(paragraph(text('[!NOTE]')), body, more);
  run(root(quote));

  assert.equal(quote.data.hProperties.type, 'note');
  assert.equal(quote.children.length, 2);
  assert.equal(quote.children[0], body);
  assert.equal(quote.children[1], more);
});

test('inline nodes after the marker are preserved', () => {
  const code = { type: 'inlineCode', value: '--tags' };
  const quote = blockquote(paragraph(text('[!WARNING]\nUse '), code, text('.')));
  run(root(quote));

  const children = quote.children[0].children;
  assert.equal(children.length, 3);
  assert.equal(children[0].value, 'Use ');
  assert.equal(children[1], code);
  assert.equal(children[2].value, '.');
});

test('alerts nested in lists and quotes are converted', () => {
  const inner = blockquote(paragraph(text('[!TIP]\nNested.')));
  const tree = root({ type: 'list', children: [{ type: 'listItem', children: [inner] }] });
  run(tree);

  assert.equal(inner.data.hProperties.type, 'tip');
});

test('plain blockquotes are left unchanged', () => {
  const quote = blockquote(paragraph(text('Just a quote.')));
  run(root(quote));

  assert.equal(quote.data, undefined);
  assert.equal(quote.children[0].children[0].value, 'Just a quote.');
});

test('unknown markers and mid-text markers are left unchanged', () => {
  const unknown = blockquote(paragraph(text('[!UNKNOWN]\nBody.')));
  const midText = blockquote(paragraph(text('See [!NOTE] for details.')));
  run(root(unknown, midText));

  assert.equal(unknown.data, undefined);
  assert.equal(unknown.children[0].children[0].value, '[!UNKNOWN]\nBody.');
  assert.equal(midText.data, undefined);
  assert.equal(midText.children[0].children[0].value, 'See [!NOTE] for details.');
});

test('empty blockquotes do not throw', () => {
  const quote = blockquote();
  run(root(quote));

  assert.equal(quote.data, undefined);
});
