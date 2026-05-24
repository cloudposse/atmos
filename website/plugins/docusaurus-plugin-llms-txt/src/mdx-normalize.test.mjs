import { test } from 'node:test';
import assert from 'node:assert/strict';

import { normalizeMdxToMarkdown } from './mdx-normalize.mjs';

async function normalize(src) {
  return (await normalizeMdxToMarkdown(src)).trim();
}

test('strips import statements', async () => {
  const out = await normalize(`import Intro from '@site/src/components/Intro'\n\nHello.`);
  assert.equal(out, 'Hello.');
});

test('unwraps <Intro>', async () => {
  const out = await normalize(`<Intro>This is the intro.</Intro>`);
  assert.equal(out, 'This is the intro.');
});

test('omits <Screengrab/>', async () => {
  const out = await normalize(`Before.\n\n<Screengrab title="x" slug="y" />\n\nAfter.`);
  assert.match(out, /^Before\.\s+After\.$/);
});

test('omits <DocCardList/>', async () => {
  const out = await normalize(`Before.\n\n<DocCardList />\n\nAfter.`);
  assert.match(out, /^Before\.\s+After\.$/);
});

test('converts <Terminal> to a fenced code block', async () => {
  const out = await normalize(`<Terminal>\n$ atmos version\n</Terminal>`);
  assert.match(out, /```shell\n\$ atmos version\n```/);
});

test('converts <File name="foo.yaml"> to fenced yaml block with filename', async () => {
  const out = await normalize(`<File name="atmos.yaml">\nfoo: bar\n</File>`);
  assert.match(out, /\*\*File:\*\*\s+`atmos\.yaml`/);
  assert.match(out, /```yaml\nfoo: bar\n```/);
});

test('flattens <Tabs>/<TabItem> into headings per tab', async () => {
  const src = `<Tabs>\n  <TabItem value="a" label="Alpha">Alpha body.</TabItem>\n  <TabItem value="b" label="Beta">Beta body.</TabItem>\n</Tabs>`;
  const out = await normalize(src);
  assert.match(out, /### Alpha/);
  assert.match(out, /Alpha body\./);
  assert.match(out, /### Beta/);
  assert.match(out, /Beta body\./);
});

test('renders <Note> as a blockquote with a Note label', async () => {
  const out = await normalize(`<Note>Watch out.</Note>`);
  assert.match(out, /^> \*\*Note\*\*\n>\n> Watch out\./m);
});

test('renders <Experimental> as a blockquote with warning', async () => {
  const out = await normalize(`<Experimental>Beta feature.</Experimental>`);
  assert.match(out, /^> ⚠️ Experimental\n>\n> Beta feature\./m);
});

test('renders <KeyPoints> as a blockquote with header', async () => {
  const out = await normalize(`<KeyPoints>\n- one\n- two\n</KeyPoints>`);
  assert.match(out, /\*\*Key points\*\*/);
  assert.match(out, /- one/);
  assert.match(out, /- two/);
});

test('renders <Step> children as a list item', async () => {
  const out = await normalize(`<Step>Do the thing.</Step>`);
  assert.match(out, /^- Do the thing\./);
});

test('renders <FAQItem> with a heading from question attr', async () => {
  const out = await normalize(`<FAQItem question="Why?">Because.</FAQItem>`);
  assert.match(out, /### Why\?/);
  assert.match(out, /Because\./);
});

test('renders <dl><dt><dd> as a bulleted definition list', async () => {
  const src = `<dl>\n  <dt>--flag</dt>\n  <dd>Description of the flag.</dd>\n  <dt>--other</dt>\n  <dd>Another description.</dd>\n</dl>`;
  const out = await normalize(src);
  assert.match(out, /- \*\*--flag\*\*/);
  assert.match(out, /Description of the flag\./);
  assert.match(out, /- \*\*--other\*\*/);
  assert.match(out, /Another description\./);
});

test('renders <ActionCard> as title + body + cta link', async () => {
  const src = `<ActionCard title="Get Started" href="/quick-start" ctaLabel="Begin">\n\nQuick guide.\n\n</ActionCard>`;
  const out = await normalize(src);
  assert.match(out, /\*\*Get Started\*\*/);
  assert.match(out, /Quick guide\./);
  assert.match(out, /\[Begin\]\(\/quick-start\)/);
});

test('unwraps unknown components and preserves their text', async () => {
  const out = await normalize(`<TotallyUnknownThing>Inner text.</TotallyUnknownThing>`);
  assert.equal(out, 'Inner text.');
});

test('strips bare mdx expressions', async () => {
  const out = await normalize(`Before\n\n{someExpression}\n\nAfter`);
  assert.match(out, /Before/);
  assert.match(out, /After/);
  assert.doesNotMatch(out, /someExpression/);
});

test('preserves vanilla markdown unchanged-ish (round-trip via remark-stringify)', async () => {
  const out = await normalize(`# Heading\n\nA paragraph with **bold** and _italics_.\n\n- one\n- two\n`);
  assert.match(out, /^# Heading/);
  assert.match(out, /\*\*bold\*\*/);
  assert.match(out, /_italics_/);
  assert.match(out, /- one/);
  assert.match(out, /- two/);
});

test('renders <SlideNotes> body and drops other slide chrome', async () => {
  const src = `<Slide>\n  <SlideTitle>Title</SlideTitle>\n  <SlideContent>Visual only.</SlideContent>\n  <SlideNotes>Speaker notes here.</SlideNotes>\n</Slide>`;
  const out = await normalize(src);
  assert.match(out, /Speaker notes here\./);
  assert.doesNotMatch(out, /Visual only/);
  assert.doesNotMatch(out, /^Title$/m);
});

test('empty input returns empty string', async () => {
  assert.equal(await normalizeMdxToMarkdown(''), '');
});
