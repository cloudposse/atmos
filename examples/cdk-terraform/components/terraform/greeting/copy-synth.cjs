const { copyFileSync } = require('node:fs');
const path = require('node:path');

copyFileSync(
  path.join(__dirname, 'cdktn.out/stacks/Demo/cdk.tf.json'),
  path.join(__dirname, 'cdk.tf.json'),
);
