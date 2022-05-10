import {createDocument} from '@stencil/core/mock-doc';
import fs from 'fs-extra';
import Listr from 'listr';
import {resolve} from 'path';

import {slugify} from '../../src/utils';

import {convertHtmlToHypertextData} from './html-to-hypertext-data';
import API from './page-types/api';
import CLI from './page-types/cli';
import Native from './page-types/native';
import Static, {toPage as toStaticPage, ToStaticPageOptions} from './page-types/static';

const tasks = new Listr(
  // { renderer: 'verbose' }
);
tasks.add(Static);
tasks.add(API);
tasks.add(CLI);
tasks.add(Native);
export default tasks;

let listrStatus: any = null;

if (!module.parent) {
  tasks.run().catch(err => {
    console.error(err);
    process.exit(1);
  });
}

export const PAGES_DIR = resolve(__dirname, '../../src/pages');

export interface Page {
  title: string;
  path: string;
  body: string;
  skipIntros?: boolean;

  [key: string]: any;
}

export type PageGetter = (status?: any) => Promise<Page[]>;

export const buildPages = async (getter: PageGetter, status?: any) => {
  // if not passed a listr status var, just set the output of an unused object
  // might be helpful for debugging
  listrStatus = status || {};
  listrStatus.output = 'Parsing Markdown';
  const pages = await getter();
  listrStatus.output = 'Optimizing';
  return Promise.all(
    pages
      .map(patchBody)
      .map(updatePageHtmlToHypertext)
      .map(writePage)
  );
};

export const buildStaticPage = async (path: string, options: ToStaticPageOptions = {}) => {
  const page = await toStaticPage(path, options);
  return writePage(updatePageHtmlToHypertext(patchBody(page)));
};

const patchBody = (page: Page): Page => {
  const body = createDocument(page.body).body;

  const h1 = body.querySelector('h1');
  if (h1 !== null && h1.textContent !== null) {
    page.title = page.title || h1.textContent.trim();
    h1.remove();
  }

  if (!page.skipIntros) {
    const children: any[] = Array.from(body.children);
    for (const child of children) {
      if (child.tagName === 'P') {
        child.classList.add('intro');
      } else {
        break;
      }
    }
  }

  const headings = Array.from(body.querySelectorAll('h2'), (heading: any) => ({
    text: heading.textContent.trim(),
    href: `#${heading.getAttribute('id')}`
  }));

  // remove /docs/ and language tag
  const prefix = /^\/docs\/([a-z]{2}\b)?/;
  const pageClass = `page-${slugify(page.path.replace(prefix, ''))}`;

  const [, language = 'en'] = prefix.exec(page.path) || [];

  if (language !== 'en') {
    if (page.previousUrl) {
      page.previousUrl = page.previousUrl.replace(prefix, `/docs/${language}/`);
    }
    if (page.nextUrl) {
      page.nextUrl = page.nextUrl.replace(prefix, `/docs/${language}/`);
    }
  }

  return {
    ...page,
    body: body.innerHTML,
    headings,
    pageClass
  };
};

export const updatePageHtmlToHypertext = (page: Page) => {
  page.body = convertHtmlToHypertextData(page.body);
  if (page.docs) {
    page.docs = convertHtmlToHypertextData(page.docs);
  }
  if (page.summary) {
    page.summary = convertHtmlToHypertextData(page.summary);
  }
  if (page.codeUsage) {
    page.codeUsage = convertHtmlToHypertextData(page.codeUsage);
  }
  if (page.usage) {
    const hypertextUsage: { [key: string]: any } = {};
    Object.keys(page.usage).forEach(key => {
      const usageContent = page.usage[key];
      hypertextUsage[key] = convertHtmlToHypertextData(usageContent);
    });
    page.usage = hypertextUsage;
  }
  return page;
};

const writePage = (page: Page): Promise<any> => {
  if (listrStatus && listrStatus._task && listrStatus._task.output !== 'Writing Pages') {
    listrStatus.output = 'Writing Pages';
  }
  return fs.outputJson(toFilePath(page.path), page, {
    spaces: 2
  });
};

const toFilePath = (urlPath: string) => {
  return `${resolve(PAGES_DIR, urlPath.replace(/.*(\/docs\/|\/pages\/)/, '') || 'index')}.json`;
};
