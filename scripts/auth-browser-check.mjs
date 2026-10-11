import { createRequire } from 'node:module';
import { spawn } from 'node:child_process';
import http from 'node:http';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(import.meta.dirname, '..');
const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'quality-auth-'));
const child = spawn(path.join(root, 'bin/quality-check'), ['-data', dir, '-gate', path.join(dir, 'gate.js'), '-owned']);
let output = '';
const info = await new Promise((resolve, reject) => {
  child.stdout.on('data', chunk => {
    output += chunk;
    if (output.includes('\n')) resolve(JSON.parse(output.split('\n')[0]));
  });
  child.once('error', reject);
});
const source = http.createServer((req, res) => {
  res.setHeader('Content-Type', 'text/html');
  res.end(`<a href="${info.url}">Open console</a>`);
});
await new Promise(resolve => source.listen(0, '127.0.0.1', resolve));
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage();
  await page.goto(`http://localhost:${source.address().port}`);
  await page.getByRole('link', { name: 'Open console' }).click();
  await page.waitForURL(info.origin + '/');
  const body = await page.locator('body').textContent();
  assert.ok(!body.includes('请使用插件生成的完整管理链接'), body);
  await page.waitForSelector('#prompt option', { state: 'attached' });
  await page.reload();
  await page.waitForSelector('#prompt option', { state: 'attached' });
  console.log('PASS: cross-site console link, ticket redirect and authenticated reload');
} finally {
  await browser.close();
  await new Promise(resolve => source.close(resolve));
  const exited = new Promise(resolve => child.once('exit', resolve));
  child.stdin.end();
  await exited;
  await fs.rm(dir, { recursive: true, force: true });
}
