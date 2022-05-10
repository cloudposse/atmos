import Listr from 'listr';

import buildData from '../build-data';
import buildMenus from '../build-menus';
import buildPages from '../build-pages';

const tasks = new Listr({collapse: false} as any);

tasks.add({
  title: 'Data',
  task: () => buildData
});

tasks.add({
  title: 'Pages',
  task: () => buildPages
});

tasks.add({
  title: 'Menus',
  task: () => buildMenus
});

tasks.run().catch((err: any) => {
  console.error(err);
  process.exit(1);
});
