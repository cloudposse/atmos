import fs from 'fs-extra';
import Listr from 'listr';
import {join, resolve} from 'path';

import {keyBy, slugify} from '../../src/utils';
import {commands} from '../data/cli.json';

const MENU_DATA_DIR = resolve(__dirname, '../../src/components/menu/data');

const cliCommandMenu = keyBy(
  commands, (item: { name: string | any[]; }) => item.name.slice(6), (item: { name: string | any[]; }) => `/docs/cli/commands/${slugify(item.name.slice(6))}`
);

const tasks = new Listr([
  {
    title: 'Build CLI command menu',
    task: () => fs.outputJSON(
      join(MENU_DATA_DIR, 'cli-commands.json'),
      cliCommandMenu,
      {spaces: 2}
    )
  }
]);

export default tasks;

if (!module.parent) {
  tasks.run().catch((err: any) => {
    console.error(err);
    process.exit(1);
  });
}
