import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, readFile, readdir, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import test from 'node:test';
import { installAtmos } from './install-atmos-bootstrap.mjs';

const binary = Buffer.from('fixture executable bytes');
const digest = createHash('sha256').update(binary).digest('hex');
async function fixture(t, runnerOS = 'Linux', runnerArch = 'X64') {
  const tempDir = await mkdtemp(join(tmpdir(), 'atmos-bootstrap-'));
  t.after(() => rm(tempDir, { recursive: true, force: true }));
  return { version: '1.223.0', runnerOS, runnerArch, tempDir, pathFile: join(tempDir, 'path'), sleep: async () => {} };
}
for (const [os, arch, target] of [
  ['Linux', 'X64', 'linux_amd64'], ['Linux', 'ARM64', 'linux_arm64'],
  ['macOS', 'X64', 'darwin_amd64'], ['macOS', 'ARM64', 'darwin_arm64'],
  ['Windows', 'X64', 'windows_amd64.exe'], ['Windows', 'ARM64', 'windows_arm64.exe'],
]) {
  test(`installs verified ${target} without release-list API calls`, async t => {
    const options = await fixture(t, os, arch);
    const requests = [];
    const asset = `atmos_1.223.0_${target}`;
    options.fetchImpl = async url => {
      requests.push(url);
      if (url.endsWith('/atmos_1.223.0_SHA256SUMS')) return new Response(`${'0'.repeat(64)}  unrelated\r\n${digest}  ${asset}\r\n`);
      assert.equal(url, `https://github.com/cloudposse/atmos/releases/download/v1.223.0/${asset}`);
      return new Response(binary);
    };
    const executable = await installAtmos(options);
    assert.deepEqual(await readFile(executable), binary);
    assert.equal(basename(executable), os === 'Windows' ? 'atmos.exe' : 'atmos');
    assert.equal(await readFile(options.pathFile, 'utf8'), `${dirname(executable)}\n`);
    assert.equal(requests.length, 2);
    if (os !== 'Windows' && process.platform !== 'win32') assert.equal((await stat(executable)).mode & 0o777, 0o755);
  });
}
for (const [name, checksum] of [['wrong checksum', '0'.repeat(64)], ['missing checksum', digest + 'invalid']]) {
  test(`rejects ${name} before installation`, async t => {
    const options = await fixture(t);
    options.fetchImpl = async url => new Response(url.endsWith('SHA256SUMS') ? `${checksum}  atmos_1.223.0_linux_amd64\n` : binary);
    await assert.rejects(installAtmos(options), /checksum/i);
    assert.deepEqual(await readdir(options.tempDir), []);
  });
}
test('rejects ambiguous checksums', async t => {
  const options = await fixture(t);
  options.fetchImpl = async () => new Response(`${digest}  atmos_1.223.0_linux_amd64\n`.repeat(2));
  await assert.rejects(installAtmos(options), /ambiguous checksum/);
});
test('retries transient failures and respects Retry-After', async t => {
  const options = await fixture(t);
  const waits = []; let calls = 0;
  options.sleep = async delay => waits.push(delay);
  options.fetchImpl = async url => {
    calls++;
    if (calls === 1) throw new Error('temporary network failure');
    if (calls === 2) return new Response('slow down', { status: 429, headers: { 'Retry-After': '12' } });
    return new Response(url.endsWith('SHA256SUMS') ? `${digest}  atmos_1.223.0_linux_amd64\n` : binary);
  };
  await installAtmos(options);
  assert.deepEqual(waits, [5000, 12000]); assert.equal(calls, 4);
});
for (const [name, status, headers, calls] of [
  ['missing release', 404, {}, 1], ['exhausted transient retries', 503, {}, 3],
  ['long server backoff', 429, { 'Retry-After': '3600' }, 1],
]) {
  test(`fails promptly on ${name}`, async t => {
    const options = await fixture(t); let count = 0;
    options.fetchImpl = async () => { count++; return new Response('failed', { status, headers }); };
    await assert.rejects(installAtmos(options), /Download failed/);
    assert.equal(count, calls); assert.deepEqual(await readdir(options.tempDir), []);
  });
}
test('rejects ranges and unsupported runners before network access', async t => {
  const options = await fixture(t);
  options.fetchImpl = async () => assert.fail('unexpected download');
  for (const version of ['latest', '1.x', '../1.2.3', '1.2.3\n', '1.2.3\nPATH=injected']) {
    await assert.rejects(installAtmos({ ...options, version }), /exact Atmos version/);
  }
  await assert.rejects(installAtmos({ ...options, runnerOS: 'unsupported' }), /Unsupported runner/);
});

test('body cleanup errors cannot bypass the maximum server backoff', async t => {
  const options = await fixture(t); const waits = []; let calls = 0;
  options.sleep = async delay => waits.push(delay);
  options.fetchImpl = async () => {
    calls++;
    const body = new ReadableStream({ start(controller) { controller.error(new Error('body failed')); } });
    return new Response(body, { status: 429, headers: { 'Retry-After': '3600' } });
  };
  await assert.rejects(installAtmos(options), /Download failed/);
  assert.equal(calls, 1); assert.deepEqual(waits, []);
  assert.deepEqual(await readdir(options.tempDir), []);
});
