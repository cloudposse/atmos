const { run } = require('./index.cjs');
run({ cleanup: true }).catch(error => {
  console.error(`DNS cleanup failed: ${error.message}`);
  process.exitCode = 1;
});
