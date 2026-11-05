const { createDiagnostics } = require('./diagnostics.cjs');

async function main(env = process.env, create = createDiagnostics) {
  if (env.RUNNER_OS !== 'Linux') throw new Error('Network diagnostics require a Linux runner');
  const boolean = name => {
    const value = env[name] || 'false';
    if (!['true', 'false'].includes(value)) throw new Error(`Invalid boolean input: ${name}`);
    return value === 'true';
  };
  await create({ env }).perform(env.INPUT_OPERATION, {
    phase: env.INPUT_PHASE,
    expectRepair: boolean('INPUT_EXPECT-REPAIR'),
    repairFirst: boolean('INPUT_REPAIR-FIRST'),
  });
}
module.exports = { main };
if (require.main === module) main().catch(error => {
  console.error(`Network diagnostics failed: ${error.message}`);
  process.exitCode = 1;
});
