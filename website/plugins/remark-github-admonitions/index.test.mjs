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

test('same-line text after the marker leaves the blockquote unchanged', () => {
  const quote = blockquote(paragraph(text('[!NOTE]worthy text')));
  const spaced = blockquote(paragraph(text('[!NOTE] same line text')));
  run(root(quote, spaced));

  assert.equal(quote.data, undefined);
  assert.equal(quote.children[0].children[0].value, '[!NOTE]worthy text');
  assert.equal(spaced.data, undefined);
  assert.equal(spaced.children[0].children[0].value, '[!NOTE] same line text');
});

test('marker followed by trailing spaces and a newline still converts', () => {
  const quote = blockquote(paragraph(text('[!TIP]  \nBody text.')));
  run(root(quote));

  assert.equal(quote.data.hProperties.type, 'tip');
  assert.equal(quote.children[0].children[0].value, 'Body text.');
});

test('a hard break after the marker is removed from the body', () => {
  // A marker line ending in two spaces or a backslash parses as text, break, text.
  const quote = blockquote(paragraph(text('[!WARNING]'), { type: 'break' }, text('Body text.')));
  run(root(quote));

  assert.equal(quote.data.hProperties.type, 'warning');
  const children = quote.children[0].children;
  assert.equal(children.length, 1);
  assert.equal(children[0].value, 'Body text.');
});

test('a break after a marker with nothing else leaves an empty blockquote removed', () => {
  const quote = blockquote(paragraph(text('[!NOTE]'), { type: 'break' }));
  run(root(quote));

  assert.equal(quote.data.hProperties.type, 'note');
  assert.equal(quote.children.length, 0);
});

test('breaks later in the body are preserved', () => {
  const quote = blockquote(paragraph(text('[!NOTE]\nLine one.'), { type: 'break' }, text('Line two.')));
  run(root(quote));

  const children = quote.children[0].children;
  assert.equal(children.length, 3);
  assert.equal(children[1].type, 'break');
});
