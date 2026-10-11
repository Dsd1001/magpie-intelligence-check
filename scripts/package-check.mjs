import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import assert from 'node:assert/strict';
const root=path.resolve(import.meta.dirname,'..');
const manifest=JSON.parse(await fs.readFile(path.join(root,'dist/manifest.json'),'utf8'));
for(const item of manifest.artifacts){
 const bytes=await fs.readFile(path.join(root,'dist',item.name));assert.equal(bytes.length,item.size);assert.equal(createHash('sha256').update(bytes).digest('hex'),item.sha256,item.name);
 if(item.name.endsWith('.tgz')){
  const listing=spawnSync('tar',['-tzf',path.join(root,'dist',item.name)],{encoding:'utf8'});assert.equal(listing.status,0);
  for(const name of ['package.json','plugin/index.mjs','plugin/gate.middleware.js','plugin/cloudflared-runtime.mjs','third_party/cloudflared/LICENSE','README.md'])assert.ok(listing.stdout.includes('/'+name),`${item.name} missing ${name}`);
  const prefix=item.name.slice(0,-4);const gate=spawnSync('tar',['-xOzf',path.join(root,'dist',item.name),prefix+'/plugin/gate.middleware.js'],{encoding:'utf8'});assert.equal(gate.stdout,'export function onRequest(body) { return body; }\n');
 }
}
console.log(`PASS: ${manifest.artifacts.length} checksummed artifacts, complete plugin archives, no runtime gate credentials`);
