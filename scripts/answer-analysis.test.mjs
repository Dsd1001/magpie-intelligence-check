import test from 'node:test';
import assert from 'node:assert/strict';
import { analyzeAnswer } from '../internal/check/web/assets/answer-analysis.js';
test('literal coverage reports missing originals without identity attribution',()=>{
 const checks=analyzeAnswer({prompt:'repeat',answer:'alpha definition'}, {text:'1. alpha\n2. beta\n'});
 assert.equal(checks.find(([name])=>name==='逐字覆盖')[1],'1/2 个条目包含完整原文');
 assert.equal(checks.find(([name])=>name==='未完整出现')[1],'beta');
 assert.match(checks.at(-1)[1],/不能识别模型身份/);
});
test('code markers are observations, failures retain completeness warning',()=>{
 const checks=analyzeAnswer({prompt:'svg',answer:'<svg></svg>',error:'truncated'});
 assert.ok(checks.some(([,value])=>value.includes('截断')));
 assert.ok(checks.some(([,value])=>value==='检测到 SVG 标记'));
 assert.deepEqual(analyzeAnswer({answer:''}),[]);
});
