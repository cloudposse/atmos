// Atmos GitHub Runtime — expose the runner's ACTIONS_* credentials to run steps.
//
// The runner injects ACTIONS_RUNTIME_TOKEN / ACTIONS_RESULTS_URL (etc.) into the
// process environment of *action* steps but NOT into `run:` shell steps. This
// JavaScript action runs in the action context, so process.env has them; it
// re-exposes them either as masked step outputs (mode=output, default) or by
// exporting every ACTIONS_* var to $GITHUB_ENV (mode=env).
//
// Zero dependencies on purpose (no @actions/core / node_modules to vendor): we
// emit workflow commands on stdout and append to the $GITHUB_OUTPUT/$GITHUB_ENV
// files directly, exactly as the toolkit does.

'use strict';

const fs = require('fs');

// Map ACTIONS_* env var names to this action's declared output names.
const OUTPUT_NAMES = {
  ACTIONS_RUNTIME_TOKEN: 'runtime-token',
  ACTIONS_RESULTS_URL: 'results-url',
  ACTIONS_CACHE_URL: 'cache-url',
  ACTIONS_RUNTIME_URL: 'runtime-url',
};

// Mask anything that looks like a credential so it never lands in logs.
function isSecret(name) {
  return name.includes('TOKEN');
}

// GitHub's $GITHUB_OUTPUT/$GITHUB_ENV file format: `name=value`, or a heredoc
// for multiline values. Always use LF (the file parser rejects CRLF).
function renderKeyValue(name, value) {
  if (value.includes('\n')) {
    const delimiter = `ghadelimiter_${name}_${Date.now()}`;
    return `${name}<<${delimiter}\n${value}\n${delimiter}\n`;
  }
  return `${name}=${value}\n`;
}

function appendToFile(pathEnvVar, content) {
  const file = process.env[pathEnvVar];
  if (!file) {
    throw new Error(`${pathEnvVar} is not set (is this running inside GitHub Actions?)`);
  }
  fs.appendFileSync(file, content);
}

function run() {
  const mode = (process.env.INPUT_MODE || 'output').trim().toLowerCase();
  if (mode !== 'output' && mode !== 'env') {
    throw new Error(`invalid mode "${mode}": expected "output" or "env"`);
  }

  let exposed = 0;
  for (const [name, value] of Object.entries(process.env)) {
    if (!name.startsWith('ACTIONS_') || !value) {
      continue;
    }
    if (isSecret(name)) {
      // Workflow command: redact this value everywhere in the logs.
      console.log(`::add-mask::${value}`);
    }

    if (mode === 'env') {
      appendToFile('GITHUB_ENV', renderKeyValue(name, value));
      exposed++;
    } else {
      const outputName = OUTPUT_NAMES[name];
      if (outputName) {
        appendToFile('GITHUB_OUTPUT', renderKeyValue(outputName, value));
        exposed++;
      }
    }
  }

  console.log(`Exposed ${exposed} ACTIONS_* value(s) via ${mode === 'env' ? '$GITHUB_ENV' : 'step outputs'}.`);
}

try {
  run();
} catch (error) {
  console.log(`::error::${error.message}`);
  process.exit(1);
}
