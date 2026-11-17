import { readFileSync, writeFileSync } from 'node:fs';

const path = 'packages/backend/package.json';
const pkg = JSON.parse(readFileSync(path, 'utf8'));
pkg.dependencies['@backstage/backend-plugin-api'] = '1.11.0';
pkg.dependencies['@backstage/plugin-scaffolder-node'] = '0.13.8';
writeFileSync(path, `${JSON.stringify(pkg, null, 2)}\n`);

const entry = 'packages/backend/src/index.ts';
const source = readFileSync(entry, 'utf8');
if (!source.includes('backend.start();')) {
  throw new Error('Generated Backstage entry point changed');
}
writeFileSync(entry, source.replace('backend.start();', "backend.add(import('./atmos'));\n\nbackend.start();"));

const appPackagePath = 'packages/app/package.json';
const appPackage = JSON.parse(readFileSync(appPackagePath, 'utf8'));
appPackage.dependencies['@fontsource-variable/geist'] = '5.3.0';
appPackage.dependencies['@fontsource-variable/geist-mono'] = '5.3.0';
writeFileSync(appPackagePath, `${JSON.stringify(appPackage, null, 2)}\n`);

// These are direct imports of our frontend modules, not transitive dependencies.
Object.assign(appPackage.dependencies, {
  '@backstage/theme': '0.7.3',
  '@backstage/plugin-catalog-react': '3.3.0',
  '@backstage/plugin-catalog-common': '1.2.1',
  '@backstage/plugin-permission-react': '0.5.6',
  '@backstage/plugin-techdocs-react': '1.4.0',
});
writeFileSync(appPackagePath, `${JSON.stringify(appPackage, null, 2)}\n`);

// The generator owns React's entry point; load our BUI tokens after its styles.
const indexPath = 'packages/app/src/index.tsx';
writeFileSync(indexPath, readFileSync(indexPath, 'utf8') + "\nimport './geist.css';\n");
