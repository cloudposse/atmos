import glob from 'fast-glob';
import frontMatter, {FrontMatterResult} from 'front-matter';
import fs from 'fs-extra';
import moment from 'moment';
import simplegit from 'simple-git/promise';

import * as GITHUB_COMMITS from '../../data/github-commits.json';
import {buildPages, Page, PAGES_DIR} from '../index';
import markdownRenderer from '../markdown-renderer';

// ignored by git
// generated in build-data/file-contrbutors.ts by build-data npm task

export default {
  title: 'Build static pages',
  task: (_: any, status: any) => buildPages(getStaticPages, status)
};

const getStaticPages = async (): Promise<Page[]> => {
  const paths = await getMarkdownPaths(PAGES_DIR);
  return Promise.all(paths.map(path => toPage(path)));
};

export const getMarkdownPaths = (cwd: string): Promise<string[]> =>
  glob('**/*.md', {
    absolute: true,
    cwd
  });

export interface ToStaticPageOptions {
  prod?: boolean;
}

export const toPage = async (path: string, {prod = true}: ToStaticPageOptions = {}) => {
  return {
    path: path.replace(PAGES_DIR, '/docs').replace(/\.md$/i, ''),
    github: prod ? await getGitHubData(path) : null,
    ...renderMarkdown(await readMarkdown(path))
  };
};

const renderMarkdown = (markdown: string) => {
  const {body, attributes} = frontMatter(markdown) as FrontMatterResult<any>;

  return {
    ...attributes,
    body: markdownRenderer(body)
  };
};

const readMarkdown = (path: string): Promise<string> =>
  fs.readFile(path, {
    encoding: 'utf8'
  });

const getGitHubData = async (filePath: string) => {
  const [, path] = /^.+\/(src\/pages\/.+\.md)$/.exec(filePath) ?? [];

  try {
    const {contributors, lastUpdated} = await getFileContributors(filePath);
    return {
      path,
      contributors,
      lastUpdated
    };
  } catch (error) {
    console.warn(error);
    return {
      path,
      contributors: [],
      lastUpdated: new Date('2019-01-23').toISOString()
    };
  }
};

const getFileContributors = async (filename: string) => {
  return simplegit().log({file: filename}).then((status: { all: any[]; latest: { date: any; }; }) => ({
      contributors: Array.from(new Set(status.all.map(commit => {
        const commits: { [key: string]: any } = GITHUB_COMMITS;
        // only add the user ID if we can find it based on the commit hash
        return commits[commit.hash] ? commits[commit.hash].id : null;
        // filter out null users
      }).filter((user: any) => !!user))),
      // tslint:disable-next-line
      lastUpdated: status.latest ? moment(status.latest.date, 'YYYY-MM-DD HH-mm-ss ZZ').toISOString() : null
    })
  );
};
