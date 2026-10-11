import {spawn} from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import assert from 'node:assert/strict';
const root=process.env.PLUGIN_ROOT||path.resolve(import.meta.dirname,'..');const dir=await fs.mkdtemp(path.join(os.tmpdir(),'quality-plugin-'));
await fs.cp(path.join(root,'plugin'),path.join(dir,'plugin'),{recursive:true});
const script=`import plugin from ${JSON.stringify('file://'+path.join(dir,'plugin/index.mjs'))};const hooks=await plugin.server({},${JSON.stringify({binaryPath:path.join(root,'bin/quality-check'),dataDir:path.join(dir,'data')})});await hooks.config({});const first=await hooks.auth.methods[0].authorize();const second=await hooks.auth.methods[0].authorize();console.log(JSON.stringify({first:first.url,second:second.url}));setInterval(()=>{},1000);`;
const p=spawn(process.execPath,['--input-type=module','-e',script],{stdio:['ignore','pipe','pipe']});let raw='';let error='';p.stderr.on('data',b=>error+=b);
try{
 const v=await new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(Error('plugin startup timeout '+error)),15000);p.stdout.on('data',b=>{raw+=b;const i=raw.indexOf('\n');if(i>=0){clearTimeout(timer);resolve(JSON.parse(raw.slice(0,i)))}});p.once('exit',()=>{clearTimeout(timer);reject(Error(error))})});
 assert.notEqual(v.first,v.second);const r=await fetch(v.first,{redirect:'manual'});assert.equal(r.status,303);const repeat=await fetch(v.first,{redirect:'manual'});assert.equal(repeat.status,401);
 p.kill('SIGKILL');await new Promise(r=>p.once('exit',r));
 await new Promise(r=>setTimeout(r,1000));let closed=false;try{await fetch(v.second)}catch{closed=true}assert.ok(closed,'Go service survived host exit');
 console.log('PASS: plugin config starts service, repeated opens issue new tickets, host death stops Go service');
}finally{if(p.exitCode===null&&p.signalCode===null)p.kill('SIGKILL');await fs.rm(dir,{recursive:true,force:true})}
