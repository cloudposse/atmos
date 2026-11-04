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

// The converted block: its class list, title text, and the body nodes after the title.
function alert(quote) {
  const [title, ...body] = quote.children;
  return {
    classes: quote.data.hProperties.className,
    titleClasses: title.data.hProperties.className,
    title: title.children[0].value,
    body,
  };
}

const TYPE_CASES = [
  ['NOTE', 'note', 'Note'],
  ['TIP', 'tip', 'Tip'],
  ['IMPORTANT', 'important', 'Important'],
  ['WARNING', 'warning', 'Warning'],
  ['CAUTION', 'caution', 'Caution'],
];

for (const [marker, type, title] of TYPE_CASES) {
  test(`[!${marker}] becomes a ${type} alert titled ${title}`, () => {
    const quote = blockquote(paragraph(text(`[!${marker}]\nBody text.`)));
    run(root(quote));

    assert.equal(quote.data.hName, 'div');
    const converted = alert(quote);
    assert.deepEqual(converted.classes, ['github-alert', `github-alert-${type}`]);
    assert.deepEqual(converted.titleClasses, ['github-alert-title']);
    assert.equal(converted.title, title);
    assert.equal(converted.body.length, 1);
    assert.equal(converted.body[0].children[0].value, 'Body text.');
  });
}

test('marker is matched case-insensitively', () => {
  const quote = blockquote(paragraph(text('[!warning]\nBody text.')));
  run(root(quote));

  assert.deepEqual(alert(quote).classes, ['github-alert', 'github-alert-warning']);
  assert.equal(alert(quote).title, 'Warning');
});

test('marker on its own paragraph is dropped and later content is kept', () => {
  const body = paragraph(text('First paragraph.'));
  const more = paragraph(text('Second paragraph.'));
  const quote = blockquote(paragraph(text('[!NOTE]')), body, more);
  run(root(quote));

  const converted = alert(quote);
  assert.deepEqual(converted.classes, ['github-alert', 'github-alert-note']);
  assert.equal(converted.body.length, 2);
  assert.equal(converted.body[0], body);
  assert.equal(converted.body[1], more);
});

test('inline nodes after the marker are preserved', () => {
  const code = { type: 'inlineCode', value: '--tags' };
  const quote = blockquote(paragraph(text('[!WARNING]\nUse '), code, text('.')));
  run(root(quote));

  const children = alert(quote).body[0].children;
  assert.equal(children.length, 3);
  assert.equal(children[0].value, 'Use ');
  assert.equal(children[1], code);
  assert.equal(children[2].value, '.');
});

test('alerts nested in lists and quotes are converted', () => {
  const inner = blockquote(paragraph(text('[!TIP]\nNested.')));
  const tree = root({ type: 'list', children: [{ type: 'listItem', children: [inner] }] });
  run(tree);

  assert.deepEqual(alert(inner).classes, ['github-alert', 'github-alert-tip']);
});

test('plain blockquotes are left unchanged', () => {
  const quote = blockquote(paragraph(text('Just a quote.')));
  run(root(quote));

  assert.equal(quote.data, undefined);
  assert.equal(quote.children.length, 1);
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

  assert.equal(alert(quote).title, 'Tip');
  assert.equal(alert(quote).body[0].children[0].value, 'Body text.');
});

test('a hard break after the marker is removed from the body', () => {
  // A marker line ending in two spaces or a backslash parses as text, break, text.
  const quote = blockquote(paragraph(text('[!WARNING]'), { type: 'break' }, text('Body text.')));
  run(root(quote));

  const children = alert(quote).body[0].children;
  assert.equal(children.length, 1);
  assert.equal(children[0].value, 'Body text.');
});

test('a marker with only a break after it leaves just the title', () => {
  const quote = blockquote(paragraph(text('[!NOTE]'), { type: 'break' }));
  run(root(quote));

  const converted = alert(quote);
  assert.equal(converted.title, 'Note');
  assert.equal(converted.body.length, 0);
});

test('breaks later in the body are preserved', () => {
  const quote = blockquote(paragraph(text('[!NOTE]\nLine one.'), { type: 'break' }, text('Line two.')));
  run(root(quote));

  const children = alert(quote).body[0].children;
  assert.equal(children.length, 3);
  assert.equal(children[1].type, 'break');
});
