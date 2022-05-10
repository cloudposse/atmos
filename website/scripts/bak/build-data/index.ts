import Listr from 'listr';

import buildSearchIndex from './search-index';

const tasks = new Listr([
    buildSearchIndex,
  ],
);

export default tasks;

if (!module.parent) {
  tasks.run().catch((err: any) => {
    console.error(err);
    process.exit(1);
  });
}
