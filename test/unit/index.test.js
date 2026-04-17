'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { mkdtempSync, readFileSync, existsSync, rmSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join } = require('node:path');

const { httpGet, githubJson, resolveReleaseTag, downloadFile } = require('../../src/index.js');

function startServer(handler) {
  return new Promise((resolve) => {
    const server = http.createServer(handler);
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      resolve({
        server,
        base: `http://127.0.0.1:${port}`,
        close: () => new Promise((r) => server.close(r)),
      });
    });
  });
}

function makeTmpDir() {
  const dir = mkdtempSync(join(tmpdir(), 'setup-php-test-'));
  return {
    dir,
    cleanup: () => rmSync(dir, { recursive: true, force: true }),
  };
}

test('httpGet returns status and body for 200', async () => {
  const { base, close } = await startServer((req, res) => {
    res.writeHead(200, { 'content-type': 'text/plain' });
    res.end('hello');
  });
  try {
    const r = await httpGet(`${base}/x`);
    assert.equal(r.statusCode, 200);
    assert.equal(r.body.toString(), 'hello');
  } finally {
    await close();
  }
});

test('httpGet follows 302 redirects', async () => {
  const { base, close } = await startServer((req, res) => {
    if (req.url === '/start') {
      res.writeHead(302, { location: `${base}/dest` });
      res.end();
      return;
    }
    res.writeHead(200);
    res.end('final');
  });
  try {
    const r = await httpGet(`${base}/start`);
    assert.equal(r.statusCode, 200);
    assert.equal(r.body.toString(), 'final');
  } finally {
    await close();
  }
});

test('httpGet rejects after too many redirects', async () => {
  const { base, close } = await startServer((req, res) => {
    res.writeHead(302, { location: `${base}/loop` });
    res.end();
  });
  try {
    await assert.rejects(() => httpGet(`${base}/loop`, { maxRedirects: 3 }), /Too many redirects/);
  } finally {
    await close();
  }
});

test('githubJson parses 200 response', async () => {
  const { base, close } = await startServer((req, res) => {
    assert.equal(req.headers['user-agent'], 'buildrush/setup-php');
    assert.equal(req.headers.accept, 'application/vnd.github+json');
    res.writeHead(200, { 'content-type': 'application/json' });
    res.end(JSON.stringify({ tag_name: 'v1.2.3' }));
  });
  try {
    const data = await githubJson('/repos/x/y/releases/latest', { apiBase: base });
    assert.equal(data.tag_name, 'v1.2.3');
  } finally {
    await close();
  }
});

test('githubJson sends bearer token when provided', async () => {
  let seen;
  const { base, close } = await startServer((req, res) => {
    seen = req.headers.authorization;
    res.writeHead(200);
    res.end('{}');
  });
  try {
    await githubJson('/anything', { apiBase: base, token: 'abc123' });
    assert.equal(seen, 'Bearer abc123');
  } finally {
    await close();
  }
});

test('githubJson throws with statusCode on 404', async () => {
  const { base, close } = await startServer((req, res) => {
    res.writeHead(404);
    res.end('Not Found');
  });
  try {
    await assert.rejects(
      () => githubJson('/missing', { apiBase: base }),
      (err) => err.statusCode === 404 && /HTTP 404/.test(err.message),
    );
  } finally {
    await close();
  }
});

test('resolveReleaseTag returns the ref when a release exists for it', async () => {
  const { base, close } = await startServer((req, res) => {
    if (req.url === '/repos/o/r/releases/tags/v1.2.3') {
      res.writeHead(200);
      res.end(JSON.stringify({ tag_name: 'v1.2.3' }));
      return;
    }
    res.writeHead(500);
    res.end('unexpected');
  });
  try {
    const tag = await resolveReleaseTag('o/r', 'v1.2.3', { apiBase: base });
    assert.equal(tag, 'v1.2.3');
  } finally {
    await close();
  }
});

test('resolveReleaseTag falls back to latest for floating major tag', async () => {
  const { base, close } = await startServer((req, res) => {
    if (req.url === '/repos/o/r/releases/tags/v1') {
      res.writeHead(404);
      res.end('Not Found');
      return;
    }
    if (req.url === '/repos/o/r/releases/latest') {
      res.writeHead(200);
      res.end(JSON.stringify({ tag_name: 'v1.4.2' }));
      return;
    }
    res.writeHead(500);
    res.end();
  });
  try {
    const tag = await resolveReleaseTag('o/r', 'v1', { apiBase: base });
    assert.equal(tag, 'v1.4.2');
  } finally {
    await close();
  }
});

test('resolveReleaseTag rejects when major mismatches latest', async () => {
  const { base, close } = await startServer((req, res) => {
    if (req.url === '/repos/o/r/releases/tags/v1') {
      res.writeHead(404);
      res.end();
      return;
    }
    if (req.url === '/repos/o/r/releases/latest') {
      res.writeHead(200);
      res.end(JSON.stringify({ tag_name: 'v2.0.0' }));
      return;
    }
    res.writeHead(500);
    res.end();
  });
  try {
    await assert.rejects(
      () => resolveReleaseTag('o/r', 'v1', { apiBase: base }),
      /No release found for v1/,
    );
  } finally {
    await close();
  }
});

test('resolveReleaseTag falls back to latest for a non-semver ref (branch/SHA)', async () => {
  const { base, close } = await startServer((req, res) => {
    if (req.url === '/repos/o/r/releases/tags/main') {
      res.writeHead(404);
      res.end();
      return;
    }
    if (req.url === '/repos/o/r/releases/latest') {
      res.writeHead(200);
      res.end(JSON.stringify({ tag_name: 'v1.4.2' }));
      return;
    }
    res.writeHead(500);
    res.end();
  });
  try {
    const tag = await resolveReleaseTag('o/r', 'main', { apiBase: base });
    assert.equal(tag, 'v1.4.2');
  } finally {
    await close();
  }
});

test('downloadFile writes response body to disk on 200', async () => {
  const { base, close } = await startServer((req, res) => {
    res.writeHead(200);
    res.end('binary-contents');
  });
  const { dir, cleanup } = makeTmpDir();
  try {
    const dest = join(dir, 'bin');
    await downloadFile(`${base}/asset`, dest);
    assert.equal(readFileSync(dest, 'utf8'), 'binary-contents');
  } finally {
    cleanup();
    await close();
  }
});

test('downloadFile rejects on 404 and does not leave a file behind', async () => {
  const { base, close } = await startServer((req, res) => {
    res.writeHead(404);
    res.end('Not Found');
  });
  const { dir, cleanup } = makeTmpDir();
  try {
    const dest = join(dir, 'bin');
    await assert.rejects(() => downloadFile(`${base}/missing`, dest), /HTTP 404/);
    assert.equal(existsSync(dest), false);
  } finally {
    cleanup();
    await close();
  }
});

test('downloadFile follows a redirect to the final asset URL', async () => {
  const { base, close } = await startServer((req, res) => {
    if (req.url === '/start') {
      res.writeHead(302, { location: `${base}/asset` });
      res.end();
      return;
    }
    res.writeHead(200);
    res.end('payload');
  });
  const { dir, cleanup } = makeTmpDir();
  try {
    const dest = join(dir, 'bin');
    await downloadFile(`${base}/start`, dest);
    assert.equal(readFileSync(dest, 'utf8'), 'payload');
  } finally {
    cleanup();
    await close();
  }
});
