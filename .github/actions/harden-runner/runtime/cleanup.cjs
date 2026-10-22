const { run } = require('./index.cjs');
try {
  run({ cleanup: true });
} catch (error) {
  console.error(`DNS cleanup failed: ${error.message}`);
  process.exitCode = 1;
}
