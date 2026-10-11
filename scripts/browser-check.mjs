import { createRequire } from 'node:module';
import { spawn } from 'node:child_process';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root=path.resolve(import.meta.dirname,'..');
const dir=await fs.mkdtemp(path.join(os.tmpdir(),'quality-ui-'));
const child=spawn(path.join(root,'bin/quality-check'),['-data',dir,'-gate',path.join(dir,'gate.js'),'-owned'],{stdio:['pipe','pipe','pipe']});
let output='';
const info=await new Promise((resolve,reject)=>{child.stdout.on('data',b=>{output+=b;const i=output.indexOf('\n');if(i>=0)resolve(JSON.parse(output.slice(0,i)))});child.once('error',reject)});
const browser=await chromium.launch({headless:true});
try{
 const context=await browser.newContext();const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(info.url);await page.waitForSelector('#prompt option',{state:'attached'});
 assert.equal(await page.locator('#prompt option').count(),7);
 await page.locator('#target input').fill('fixture/solo');await page.locator('#target button').last().click();await page.waitForSelector('#targets tr');
 await page.locator('#prompt').selectOption('repeat');assert.match(await page.locator('#promptText').textContent(),/StarSrvGroupBody/);
 await fs.mkdir(path.join(root,'artifacts'),{recursive:true});
 for(const [name,width,height]of [['desktop',1280,900],['mobile',390,844]]){
  await page.setViewportSize({width,height});await page.screenshot({path:path.join(root,'artifacts',name+'.png'),fullPage:true});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`${name} overflow`);
 }
 assert.deepEqual(errors,[]);console.log('PASS: seven prompts, target creation, desktop/mobile layout, no browser errors');
 assert.equal(await page.locator('#message').textContent(),'','页面显示错误');
}finally{await browser.close();child.stdin.end();await new Promise(r=>child.once('exit',r));await fs.rm(dir,{recursive:true,force:true})}
