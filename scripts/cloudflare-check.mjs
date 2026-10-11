import {spawn} from 'node:child_process';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import {resolveCloudflared} from '../plugin/cloudflared-runtime.mjs';
import {waitForConsole} from '../plugin/console-readiness.mjs';
const root=path.resolve(import.meta.dirname,'..');const dir=await fs.mkdtemp(path.join(os.tmpdir(),'quality-cloudflare-'));
const binary=await resolveCloudflared({},process.env.CLOUDFLARED_CACHE||'/tmp/quality-cloudflare-runtime');
const child=spawn(path.join(root,'bin/quality-check'),['-data',dir,'-gate',path.join(dir,'gate.js'),'-owned'],{stdio:['pipe','pipe','pipe']});
let output='';const info=await new Promise((resolve,reject)=>{child.stdout.on('data',b=>{output+=b;const i=output.indexOf('\n');if(i>=0)resolve(JSON.parse(output.slice(0,i)))});child.once('error',reject)});
try{
 const response=await fetch(info.origin+'/api/console',{method:'POST',headers:{authorization:'Bearer '+info.control,'content-type':'application/json'},body:JSON.stringify({remote:true,binary})});
 assert.equal(response.status,200,await response.clone().text());const {url}=await response.json();
 await waitForConsole(url);
 let session;const deadline=Date.now()+120000;while(Date.now()<deadline){try{const r=await fetch(url,{redirect:'manual'});if(r.status===303){session=r.headers.get('set-cookie')?.split(';')[0];break}}catch{}await new Promise(r=>setTimeout(r,3000))}
 assert.ok(session,'temporary HTTPS domain never became reachable');const origin=new URL(url).origin;
 let state;const stableUntil=Date.now()+90000;while(Date.now()<stableUntil){try{state=await fetch(origin+'/api/state',{headers:{cookie:session},signal:AbortSignal.timeout(10000)});if(state.status===200)break}catch{}await new Promise(r=>setTimeout(r,3000))}assert.equal(state?.status,200);assert.equal((await state.json()).state.config.gateway_key,'');
 const close=await fetch(origin+'/api/console/close',{method:'POST',headers:{cookie:session,origin,'content-type':'application/json'},body:'{}'});assert.equal(close.status,200);
 await new Promise(r=>setTimeout(r,600));const invalid=await fetch(info.origin+'/api/state',{headers:{cookie:session}});assert.equal(invalid.status,401);
 console.log('PASS: actual Cloudflare HTTPS ticket exchange, authenticated state, manual close and session revocation');
}finally{child.stdin.end();await new Promise(r=>child.once('exit',r));await fs.rm(dir,{recursive:true,force:true})}
