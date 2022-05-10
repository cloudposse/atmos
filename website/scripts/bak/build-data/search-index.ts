import glob from 'fast-glob';
import {outputJson, readJson} from 'fs-extra';
import {resolve} from 'path';

const PAGES_PATH = resolve(__dirname, '../../src/pages');
const INDEX_PATH = resolve(__dirname, '../../src/components/search/data/index.json');

export default {
  title: 'Build search index',
  task: () => buildIndex(PAGES_PATH),
  skip: () => true
};

const buildIndex = async (dir: any) => {
  const paths = await getPaths(dir);
  const records = await Promise.all(paths.map(toRecord));
  return outputJson(
    INDEX_PATH,
    records,
    {spaces: 2}
  );
};

const toRecord = async (path: any) => {
  const {title} = await readJson(path);
  const href = toHref(path);
  return {
    title,
    href,
    type: 'page'
  };
};

const getPaths = (cwd: any) => {
  return glob('**/*.json', {
    absolute: true,
    cwd
  });
};

const toHref = (path: string) => {
  const [, page] = /\/pages\/(.+)\.json$/.exec(path) ?? [];
  return `/docs/${page}`;
};
