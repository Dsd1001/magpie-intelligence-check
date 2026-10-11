import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';

export async function discoverGateway(options = {}, env = process.env, fetcher = fetch) {
  let url = options.gatewayURL;
  const key = options.gatewayKey || 'magpie';
  if (!url) {
    let port = 3425;
    try {
      const settings = JSON.parse(await fs.readFile(path.join(env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config'), 'magpie', 'settings.json'), 'utf8'));
      if (Number.isInteger(settings.port) && settings.port > 0 && settings.port < 65536) port = settings.port;
    } catch {}
    const address = env.MAGPIE_ADDR || `127.0.0.1:${port}`;
    // Discovery never sends the default key to a non-loopback host.
    if (!/^(?:127\.0\.0\.1|localhost|\[::1\]):\d+$/.test(address)) return undefined;
    url = `http://${address}`;
  }
  try {
    const base = url.replace(/\/$/, '');
    const response = await fetcher(base + (base.endsWith('/v1') ? '' : '/v1') + '/models', {
      headers: { authorization: `Bearer ${key}` }, redirect: 'error', signal: AbortSignal.timeout(3000),
    });
    if (!response.ok) return undefined;
    const data = await response.json();
    if (!Array.isArray(data.data)) return undefined;
    return { gateway_url: url, gateway_key: key, log_path: options.logPath || '', daily_limit: 120 };
  } catch { return undefined; }
}
