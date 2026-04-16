'use strict';

const { execFileSync } = require('child_process');
const { existsSync, mkdirSync, chmodSync, createWriteStream } = require('fs');
const { join } = require('path');
const https = require('https');

const TOOL_CACHE = process.env.RUNNER_TOOL_CACHE || join(process.env.HOME, '.buildrush', 'cache');
const OS = process.env.RUNNER_OS?.toLowerCase() || 'linux';
const ARCH = process.env.RUNNER_ARCH === 'ARM64' ? 'arm64' : 'amd64';

async function downloadFile(url, dest) {
  return new Promise((resolve, reject) => {
    const file = createWriteStream(dest);
    https
      .get(url, { headers: { 'User-Agent': 'buildrush/setup-php' } }, (res) => {
        if (res.statusCode === 302 || res.statusCode === 301) {
          https
            .get(res.headers.location, (redirectRes) => {
              redirectRes.pipe(file);
              file.on('finish', () => {
                file.close();
                resolve();
              });
            })
            .on('error', reject);
          return;
        }
        res.pipe(file);
        file.on('finish', () => {
          file.close();
          resolve();
        });
      })
      .on('error', reject);
  });
}

async function main() {
  // Determine binary name
  const binaryName = OS === 'windows' ? 'phpup.exe' : 'phpup';
  const platform = OS === 'macos' ? 'darwin' : OS;
  const releaseBinary = `phpup-${platform}-${ARCH}`;

  // Check tool cache for binary
  const cacheDir = join(TOOL_CACHE, 'buildrush-bin');
  const cachedBinary = join(cacheDir, binaryName);

  if (!existsSync(cachedBinary)) {
    // Download from this action's release
    const actionRef = process.env.GITHUB_ACTION_REF || 'main';
    const actionRepo = process.env.GITHUB_ACTION_REPOSITORY || 'buildrush/setup-php';
    const releaseUrl = `https://github.com/${actionRepo}/releases/download/${actionRef}/${releaseBinary}`;

    console.log(`Downloading phpup binary from ${releaseUrl}`);
    mkdirSync(cacheDir, { recursive: true });
    await downloadFile(releaseUrl, cachedBinary);
    chmodSync(cachedBinary, 0o755);
  }

  // Execute phpup with all inputs forwarded via env
  try {
    execFileSync(cachedBinary, [], {
      stdio: 'inherit',
      env: process.env,
    });
  } catch (err) {
    process.exitCode = err.status || 1;
  }
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
