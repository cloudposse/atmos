import { createHash } from 'node:crypto';
import { appendFile, chmod, mkdir, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout } from 'node:timers/promises';

// Exact release assets avoid listing releases through GitHub's shared API quota.
// Both the executable and its published checksum come from the pinned release.
export async function installAtmos({ version, runnerOS, runnerArch, tempDir, pathFile,
  fetchImpl = fetch, sleep = setTimeout }) {
  version = version?.replace(/^v/, '');
  if (!/^\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?$/.test(version ?? '') || version.trim() !== version) {
    throw new Error('Bootstrap requires an exact Atmos version.');
  }
  const os = { Linux: 'linux', macOS: 'darwin', Windows: 'windows' }[runnerOS];
  const arch = { X64: 'amd64', ARM64: 'arm64' }[runnerArch];
  if (!os || !arch || !tempDir || !pathFile) {
    throw new Error('Unsupported runner or missing RUNNER_TEMP/GITHUB_PATH.');
  }
  const extension = os === 'windows' ? '.exe' : '';
  const asset = `atmos_${version}_${os}_${arch}${extension}`;
  const base = `https://github.com/cloudposse/atmos/releases/download/v${version}`;
  const checksums = (await download(`${base}/atmos_${version}_SHA256SUMS`, fetchImpl, sleep)).toString('utf8');
  const matches = checksums.split(/\r?\n/).map(line => line.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/))
    .filter(match => match?.[2] === asset);
  if (matches.length !== 1) throw new Error(`Missing or ambiguous checksum for ${asset}.`);
  const binary = await download(`${base}/${asset}`, fetchImpl, sleep);
  const digest = createHash('sha256').update(binary).digest('hex');
  if (digest !== matches[0][1].toLowerCase()) throw new Error(`Checksum mismatch for ${asset}.`);

  // Publish to PATH only after verification; never execute unverified content.
  const directory = join(tempDir, `atmos-bootstrap-${version}-${os}-${arch}`);
  const executable = join(directory, `atmos${extension}`);
  await mkdir(directory, { recursive: true });
  await writeFile(executable, binary);
  if (os !== 'windows') await chmod(executable, 0o755);
  await appendFile(pathFile, `${directory}\n`);
  return executable;
}

async function download(url, fetchImpl, sleep) {
  for (let attempt = 0; ; attempt++) {
    let delay = 5000 * (attempt + 1);
    try {
      const response = await fetchImpl(url, { signal: AbortSignal.timeout(60000) });
      if (response.ok) return Buffer.from(await response.arrayBuffer());
      const retryAfter = response.headers.get('retry-after');
      if (retryAfter) {
        const seconds = Number(retryAfter);
        delay = Number.isFinite(seconds) ? seconds * 1000 : Date.parse(retryAfter) - Date.now();
        delay = Math.max(5000, delay || 5000);
      }
      try {
        await response.body?.cancel();
      } catch {
        // Cleanup must not override the HTTP status or the server's backoff limit.
      }
      const error = new Error(`Download failed (${response.status}): ${url}`);
      // Do not retry missing releases or wait past the bounded install window.
      error.permanent = (response.status < 500 && ![408, 429].includes(response.status)) || delay > 60000;
      throw error;
    } catch (error) {
      if (error.permanent || attempt === 2) throw error;
      await sleep(delay);
    }
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    await installAtmos({ version: process.env.ATMOS_BOOTSTRAP_VERSION, runnerOS: process.env.RUNNER_OS,
      runnerArch: process.env.RUNNER_ARCH, tempDir: process.env.RUNNER_TEMP, pathFile: process.env.GITHUB_PATH });
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
