import { createRequire } from 'node:module';
import { spawn } from 'node:child_process';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import http from 'node:http';
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root=path.resolve(import.meta.dirname,'..');
const dir=await fs.mkdtemp(path.join(os.tmpdir(),'quality-ui-'));
const child=spawn(path.join(root,'bin/quality-check'),['-data',dir,'-gate',path.join(dir,'gate.js'),'-owned'],{stdio:['pipe','pipe','pipe']});
let output='';
const info=await new Promise((resolve,reject)=>{child.stdout.on('data',b=>{output+=b;const i=output.indexOf('\n');if(i>=0)resolve(JSON.parse(output.slice(0,i)))});child.once('error',reject)});
const browser=await chromium.launch({headless:true});
const upstream=http.createServer(async(req,res)=>{
 let raw='';for await(const chunk of req)raw+=chunk;
 const body=JSON.parse(raw);const count=Number(body.messages[0].content.match(/ (\d+) 个 1 到 355/)?.[1]);
 const numbers=Array.from({length:count},(_,i)=>(i*73%355)+1);
 res.setHeader('Content-Type','application/json');
 res.end(JSON.stringify({choices:[{message:{content:numbers.join(',')},finish_reason:'stop'}],usage:{total_tokens:count}}));
});
await new Promise(resolve=>upstream.listen(0,'127.0.0.1',resolve));
try{
 const context=await browser.newContext();const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(info.url);await page.waitForSelector('#prompt option',{state:'attached'});
 assert.equal(await page.locator('#prompt option').count(),7);
 await page.locator('#target input').fill('fixture/solo');await page.locator('#target button').last().click();await page.waitForSelector('#targets tr');
 await page.locator('#prompt').selectOption('repeat');assert.match(await page.locator('#promptText').textContent(),/StarSrvGroupBody/);
 await page.evaluate(async gateway=>{await fetch('/api/config',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({gateway_url:gateway,gateway_key:'fixture',daily_limit:120})})},`http://127.0.0.1:${upstream.address().port}`);
 await page.locator('#traceRun').click();
 await page.waitForFunction(()=>document.querySelector('#traceResults').textContent.includes('最相似候选'),{},{timeout:15000});
 await page.locator('#traceResults summary').click();
 assert.equal(await page.locator('#traceResults tbody tr').count(),18);
 assert.match(await page.locator('#targets').textContent(),/未检测/);
 let failNextState=true;
 await page.route('**/api/state',async route=>{
  if(failNextState){failNextState=false;await route.fulfill({status:502,contentType:'text/html',body:'<!DOCTYPE html><html>Cloudflare bad gateway</html>'});}
  else await route.continue();
 });
 await page.waitForFunction(()=>document.querySelector('#message').textContent.includes('HTTP 502'),{},{timeout:10000});
 assert.ok(!(await page.locator('#message').textContent()).includes('<html>'));
 await page.waitForFunction(()=>document.querySelector('#message').textContent==='',{},{timeout:10000});
 await page.unroute('**/api/state');
 await fs.mkdir(path.join(root,'artifacts'),{recursive:true});
 for(const [name,width,height]of [['desktop',1280,900],['mobile',390,844]]){
  await page.setViewportSize({width,height});await page.screenshot({path:path.join(root,'artifacts',name+'.png'),fullPage:true});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`${name} overflow`);
 }
 assert.deepEqual(errors,[]);console.log('PASS: seven prompts, ModelTrace batch and ranking, unchanged gate, desktop/mobile layout, no browser errors');
 assert.equal(await page.locator('#message').textContent(),'','页面显示错误');
}finally{await browser.close();await new Promise(resolve=>upstream.close(resolve));const exited=new Promise(r=>child.once('exit',r));child.stdin.end();await exited;await fs.rm(dir,{recursive:true,force:true})}
