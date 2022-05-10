import chokidar from 'chokidar';
import ora from 'ora';
import {resolve} from 'path';

import {buildStaticPage} from '../build-pages';

const PAGES_GLOB = `${resolve(__dirname, '../../src/pages')}/**/*.md`;
const watcher = chokidar.watch(PAGES_GLOB, {ignoreInitial: true});
const spinner = ora('Watching pages').start();

const handleChange = async (path: any) => {
  try {
    spinner.text = `Building ${path}`;
    await buildStaticPage(path, {prod: false});
    spinner.succeed(`Built ${path}`);
    spinner.start('Watching pages');
  } catch (err: any) {
    spinner.fail(`Failed to build ${path}`);
    console.error(err.message);
  }
};

watcher.on('add', handleChange);
watcher.on('change', handleChange);
