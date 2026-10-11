import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import os from 'node:os';
import { resolveCloudflared } from './cloudflared-runtime.mjs';
import { discoverGateway } from './gateway-discovery.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
let child, ready;
function start(options = {}) {
  if (ready) return ready;
  ready = new Promise((resolve, reject) => {
    const binary = options.binaryPath || path.join(root, 'bin', process.platform === 'win32' ? 'quality-check.exe' : 'quality-check');
    const args=['-data', options.dataDir || path.join(os.homedir(), '.config', 'magpie-quality-check'), '-gate', path.join(root, 'plugin', 'gate.middleware.js'),'-owned'];
    if(options.cloudflaredPath)args.push('-cloudflared',options.cloudflaredPath);
    child = spawn(binary, args, { stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
    child.stdin.on('error',()=>{});
    let output = '';
    const timer = setTimeout(() => { child.kill(); reject(new Error('质量检测后台启动超时')); }, 15000);
    child.stdout.on('data', b => { output += b; const i = output.indexOf('\n'); if (i >= 0) { try { const v = JSON.parse(output.slice(0, i)); clearTimeout(timer); resolve(v); } catch { reject(new Error('后台启动信息无效')); } } });
    child.stderr.on('data', () => {});
    child.once('error', e => { clearTimeout(timer); ready = undefined; reject(new Error(`无法启动 Go 后台，请先构建二进制：${e.message}`)); });
    child.once('exit', () => { clearTimeout(timer); ready = undefined; reject(new Error('检测后台已退出')); });
  });
  return ready;
}
process.once('exit', () => child?.kill());
export default { id: 'quality-check', server: async (_input, options = {}) => ({
  auth: {
    provider: 'quality-check',
    methods: [false,true].map(remote=>({ type: 'oauth', label: remote?'打开临时远程控制台（Cloudflare）':'打开本机质量检测控制台', async authorize() {
      const v = await start(options);
      const binary=remote?await resolveCloudflared(options,options.dataDir || path.join(os.homedir(), '.config', 'magpie-quality-check')):undefined;
      const r=await fetch(v.origin+'/api/console',{method:'POST',headers:{authorization:'Bearer '+v.control,'content-type':'application/json'},body:JSON.stringify({remote,binary})});
      if(!r.ok)throw Error(await r.text());const {url}=await r.json();
      return { url, instructions: remote?'临时 HTTPS 入口最长 30 分钟，一次性链接十分钟有效。页面可结束远程访问。':'一次性链接十分钟有效；使用后重新从插件打开可获取新入口。', method: 'auto', callback: async () => ({ type: 'success', key: 'quality-control-only' }) };
    } })),
    async loader() { await start(options); return {}; }
  },
  async config(cfg) {
    // The control-only provider has no inference calls to trigger loader.
    // Start on host configuration so persisted monitoring works after restart.
    await start(options);
    const service=await ready;
    const headers={authorization:'Bearer '+service.control,'content-type':'application/json'};
    const state=await fetch(service.origin+'/api/state',{headers}).then(r=>r.json());
    if(!state.state.config.gateway_url){
      const discovered=await discoverGateway(options);
      if(discovered){const saved=await fetch(service.origin+'/api/config',{method:'POST',headers,body:JSON.stringify(discovered)});if(!saved.ok)throw Error('无法保存已验证的 Magpie 网关连接');}
    }
    cfg.provider ??= {};
    cfg.provider['quality-check'] = { name: '模型质量检查', models: {} };
  }
}) };
