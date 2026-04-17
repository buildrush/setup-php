'use strict';

const { execFileSync } = require('child_process');
const { existsSync, mkdirSync, chmodSync, createWriteStream, unlinkSync } = require('fs');
const { join } = require('path');
const http = require('http');
const https = require('https');

const USER_AGENT = 'buildrush/setup-php';
const DEFAULT_API_BASE = 'https://api.github.com';
const DEFAULT_REPO = 'buildrush/setup-php';

function requestLib(url) {
  return new URL(url).protocol === 'http:' ? http : https;
}

function httpGet(url, { headers = {}, maxRedirects = 5 } = {}) {
  return new Promise((resolve, reject) => {
    const follow = (current, redirects) => {
      if (redirects > maxRedirects) {
        reject(new Error(`Too many redirects fetching ${url}`));
        return;
      }
      requestLib(current)
        .get(current, { headers: { 'User-Agent': USER_AGENT, ...headers } }, (res) => {
          const status = res.statusCode;
          if (
            (status === 301 || status === 302 || status === 307 || status === 308) &&
            res.headers.location
          ) {
            res.resume();
            const next = new URL(res.headers.location, current).toString();
            follow(next, redirects + 1);
            return;
          }
          const chunks = [];
          res.on('data', (c) => chunks.push(c));
          res.on('end', () =>
            resolve({
              statusCode: status,
              headers: res.headers,
              body: Buffer.concat(chunks),
            }),
          );
          res.on('error', reject);
        })
        .on('error', reject);
    };
    follow(url, 0);
  });
}

async function githubJson(path, { token, apiBase = DEFAULT_API_BASE } = {}) {
  const headers = {
    Accept: 'application/vnd.github+json',
    'X-GitHub-Api-Version': '2022-11-28',
  };
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await httpGet(`${apiBase}${path}`, { headers });
  if (res.statusCode < 200 || res.statusCode >= 300) {
    const snippet = res.body.toString('utf8').slice(0, 200);
    const err = new Error(`GitHub API ${path} returned HTTP ${res.statusCode}: ${snippet}`);
    err.statusCode = res.statusCode;
    throw err;
  }
  return JSON.parse(res.body.toString('utf8'));
}

async function resolveReleaseTag(repo, ref, { token, apiBase } = {}) {
  const isFloatingMajor = /^v\d+$/.test(ref || '');
  const isSpecificVersion = /^v\d+\.\d+\.\d+/.test(ref || '');

  // Floating major tags (v1, v2) are a git-ref convenience for `uses:` syntax only.
  // They must never have a corresponding GitHub Release; resolve via latest + major-match
  // so a stray major-tag release can't hijack resolution.
  if (ref && !isFloatingMajor) {
    try {
      const release = await githubJson(`/repos/${repo}/releases/tags/${encodeURIComponent(ref)}`, {
        token,
        apiBase,
      });
      return release.tag_name;
    } catch (err) {
      if (err.statusCode !== 404) throw err;
      if (isSpecificVersion) {
        throw new Error(`No release found for pinned version ${ref}`);
      }
      // Branch/SHA ref: fall through to latest (development use case).
    }
  }

  const latest = await githubJson(`/repos/${repo}/releases/latest`, {
    token,
    apiBase,
  });
  if (isFloatingMajor) {
    const majorMatch = /^v(\d+)$/.exec(ref);
    const latestMajor = /^v(\d+)/.exec(latest.tag_name);
    if (!latestMajor || latestMajor[1] !== majorMatch[1]) {
      throw new Error(`No release found for ${ref}; latest release is ${latest.tag_name}`);
    }
  }
  return latest.tag_name;
}

async function downloadFile(url, dest, { maxRedirects = 5 } = {}) {
  return new Promise((resolve, reject) => {
    const follow = (current, redirects) => {
      if (redirects > maxRedirects) {
        reject(new Error(`Too many redirects fetching ${url}`));
        return;
      }
      requestLib(current)
        .get(current, { headers: { 'User-Agent': USER_AGENT } }, (res) => {
          const status = res.statusCode;
          if (
            (status === 301 || status === 302 || status === 307 || status === 308) &&
            res.headers.location
          ) {
            res.resume();
            const next = new URL(res.headers.location, current).toString();
            follow(next, redirects + 1);
            return;
          }
          if (status < 200 || status >= 300) {
            res.resume();
            reject(new Error(`Download failed: ${current} returned HTTP ${status}`));
            return;
          }
          const file = createWriteStream(dest);
          let failed = false;
          const fail = (err) => {
            if (failed) return;
            failed = true;
            file.destroy();
            try {
              unlinkSync(dest);
            } catch {
              /* ignore */
            }
            reject(err);
          };
          res.on('error', fail);
          file.on('error', fail);
          res.pipe(file);
          file.on('finish', () => file.close((err) => (err ? fail(err) : resolve())));
        })
        .on('error', reject);
    };
    follow(url, 0);
  });
}

async function runMain({ env = process.env } = {}) {
  const osName = (env.RUNNER_OS || 'linux').toLowerCase();
  const arch = env.RUNNER_ARCH === 'ARM64' ? 'arm64' : 'amd64';
  const toolCache = env.RUNNER_TOOL_CACHE || join(env.HOME || '/tmp', '.buildrush', 'cache');
  const binaryName = osName === 'windows' ? 'phpup.exe' : 'phpup';
  const platform = osName === 'macos' ? 'darwin' : osName;
  const releaseBinary = `phpup-${platform}-${arch}`;

  const cacheDir = join(toolCache, 'buildrush-bin');
  const cachedBinary = join(cacheDir, binaryName);

  if (!existsSync(cachedBinary)) {
    const repo = env.GITHUB_ACTION_REPOSITORY || DEFAULT_REPO;
    const ref = env.GITHUB_ACTION_REF || '';
    const token = env.INPUT_GITHUB_TOKEN || env.GITHUB_TOKEN || '';

    console.log(`Resolving release tag for ${repo}@${ref || '(unknown ref, will use latest)'}`);
    const tag = await resolveReleaseTag(repo, ref, { token });
    const releaseUrl = `https://github.com/${repo}/releases/download/${tag}/${releaseBinary}`;

    console.log(`Downloading phpup binary from ${releaseUrl}`);
    mkdirSync(cacheDir, { recursive: true });
    await downloadFile(releaseUrl, cachedBinary);
    chmodSync(cachedBinary, 0o755);
  }

  try {
    execFileSync(cachedBinary, [], {
      stdio: 'inherit',
      env,
    });
  } catch (err) {
    process.exitCode = err.status || 1;
  }
}

module.exports = {
  httpGet,
  githubJson,
  resolveReleaseTag,
  downloadFile,
  runMain,
};

if (require.main === module) {
  runMain().catch((err) => {
    console.error(err);
    process.exitCode = 1;
  });
}
