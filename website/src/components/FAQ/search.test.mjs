import assert from 'node:assert/strict';
import test from 'node:test';
import { readFile } from 'node:fs/promises';
import { createFAQController, matchesQuery } from './search.mjs';
import { normalizeMdxToMarkdown } from '../../../plugins/docusaurus-plugin-llms-txt/src/mdx-normalize.mjs';

// Minimal DOM interface for testing disclosure state independently of React.
function fixture() {
  function item(id, question, answer, open = false) {
    return { id, open, hidden: false, querySelector: (selector) => ({ textContent: selector === '[data-faq-question]' ? question : answer }) };
  }
  const terraform = item('terraform', 'Do I need to rewrite?', 'Keep existing Terraform root modules.', true);
  const skills = item('skills', 'Can an agent help?', 'Install the migration skill.');
  const about = item('about', 'Is Atmos free?', 'Apache 2.0 license.');
  const items = [terraform, skills, about];
  const sections = [
    { id: 'start', hidden: false, querySelectorAll: () => [terraform, skills] },
    { id: 'general', hidden: false, querySelectorAll: () => [about] },
  ];
  const root = { querySelectorAll: (selector) => selector === '[data-faq-item]' ? items : sections };
  return { controller: createFAQController(root), terraform, skills, about, sections };
}

test('search matches all words, regardless of case, order, or surrounding whitespace', () => {
  assert.equal(matchesQuery('Use your existing Terraform root modules.', ' MODULES  terraform '), true);
  assert.equal(matchesQuery('Use your existing Terraform root modules.', 'terraform subscription'), false);
  assert.equal(matchesQuery('Use your existing Terraform root modules.', '  '), true);
});

test('search indexes closed answer text, opens matches, and hides empty categories', () => {
  const { controller, skills, terraform, sections } = fixture();
  assert.equal(controller.search('MIGRATION skill'), 1);
  assert.equal(skills.open, true);
  assert.equal(skills.hidden, false);
  assert.equal(terraform.hidden, true);
  assert.deepEqual(sections.map((section) => section.hidden), [false, true]);
});

test('clearing successive searches restores the expansion state from before searching', () => {
  const { controller, terraform, skills, about, sections } = fixture();
  controller.search('migration');
  skills.open = false;
  assert.equal(controller.search('unmatched query'), 0);
  assert.ok(sections.every((section) => section.hidden));
  controller.search('license');
  assert.equal(about.open, true);
  assert.equal(controller.search(''), 3);
  assert.deepEqual([terraform.open, skills.open, about.open], [true, false, false]);
  assert.ok(sections.every((section) => !section.hidden));
  skills.open = true;
  controller.search('license');
  controller.search(' ');
  assert.equal(skills.open, true);
});

test('question and category fragments reveal targets hidden by a search', () => {
  const { controller, skills, sections } = fixture();
  controller.search('license');
  assert.equal(controller.reveal('#%73kills'), skills);
  assert.equal(skills.open, true);
  assert.equal(skills.hidden, false);
  controller.search('license');
  assert.equal(controller.reveal('#start'), sections[0]);
  assert.equal(sections[0].hidden, false);
  assert.equal(controller.reveal('#unknown'), null);
  assert.equal(controller.reveal('#%invalid'), null);
});

test('FAQ Markdown export retains every category, question, answer, code example, and adoption link', async () => {
  const source = await readFile(new URL('../../../docs/intro/faq.mdx', import.meta.url), 'utf8');
  const body = source.replace(/^---[\s\S]*?---\s*/, '');
  const markdown = await normalizeMdxToMarkdown(body);
  const questions = Array.from(source.matchAll(/question="([^"]+)"/g), (match) => match[1]);
  const categories = Array.from(source.matchAll(/<FAQSection[^>]*title="([^"]+)"/g), (match) => match[1]);
  assert.ok(questions.length > 0);
  assert.equal(categories.length, 6);
  for (const question of questions) assert.ok(markdown.includes(`### ${question}`), question);
  for (const category of categories) assert.ok(markdown.includes(`## ${category}`), category);
  assert.match(markdown, /You can use your existing Terraform with Atmos today/);
  assert.match(markdown, /atmos ai skill install atmos-migration/);
  assert.match(markdown, /command: tofu/);
  assert.match(markdown, /!aws\.cloudformation\.output/);
  assert.match(markdown, /atmos terraform apply --all -s dev/);
  assert.match(markdown, /atmos terraform destroy --affected/);
  assert.match(markdown, /\/migration\/native-terraform/);
  assert.match(markdown, /\/ai\/skills/);
  assert.doesNotMatch(markdown, /<FAQ/);
});
