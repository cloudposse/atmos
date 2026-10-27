const { spawnSync } = require('node:child_process');

// Keep subprocess failures as data so diagnostics can record failed connections.
function execute(command, args, timeout = 5000) {
  const result = spawnSync(command, args, {
    encoding: 'utf8', timeout, killSignal: 'SIGKILL', maxBuffer: 4 * 1024 * 1024,
    shell: false,
  });
  return {
    code: result.status ?? 1,
    stdout: result.stdout || '',
    stderr: result.error?.message || result.stderr || '',
  };
}
module.exports = { execute };
