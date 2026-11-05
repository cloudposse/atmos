const { prepare } = require('./policy.cjs');
try {
  prepare();
} catch (error) {
  console.error(`Runner policy failed: ${error.message}`);
  process.exitCode = 1;
}
