import { $, text, icon, hydrateIcons, syncSelect, dateTime, names } from './ui.js';
import { renderModelTrace, runModelTrace } from '../modeltrace/panel.js';

let data, polling = false, stopped = false, pollError = '', targetSignature, historySignature, historyLimit = 30;
const config = $('#config');
const secretType = 'pass' + 'word';
config.elements.gateway_key.type = secretType;
config.elements.gateway_key.autocomplete = 'new-' + secretType;
let configDirty = false;
hydrateIcons();

function feedback(value, success = false) {
  $('#message').textContent = value;
  $('#message').className = 'feedback' + (success ? ' success' : '');
}
async function request(path, body) {
  const response = await fetch(path, {
    method: body === undefined ? 'GET' : 'POST',
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    const raw = await response.text();
    if (response.status === 401) throw Error('管理会话已失效，请从 Magpie 插件重新打开控制台。');
    if (/<!doctype|<html/i.test(raw)) throw Error(`控制台连接暂时不可用（HTTP ${response.status}），请稍后重试；持续失败请从插件重新打开远程入口。`);
    throw Error(raw.trim() || `请求失败（HTTP ${response.status}）`);
  }
  return response.json();
}
async function pending(button, work, output = $('#message')) {
  if (button.disabled) return;
  button.disabled = true;
  button.setAttribute('aria-busy', 'true');
  try { await work(); }
  catch (error) { output.textContent = error.message; output.classList.remove('success'); output.classList.add('error'); }
  finally { button.disabled = false; button.removeAttribute('aria-busy'); renderControls(); }
}
function selectView(name, focus = false) {
  document.querySelectorAll('[role=tab]').forEach(tab => {
    const active = tab.dataset.view === name;
    tab.setAttribute('aria-selected', String(active));
    tab.tabIndex = active ? 0 : -1;
    $(`#view-${tab.dataset.view}`).hidden = !active;
    if (active && focus) tab.focus();
  });
}
const tabs = [...document.querySelectorAll('[role=tab]')];
tabs.forEach((tab, i) => {
  tab.onclick = () => selectView(tab.dataset.view);
  tab.onkeydown = event => {
    const index = { ArrowRight: (i + 1) % tabs.length, ArrowLeft: (i + tabs.length - 1) % tabs.length, Home: 0, End: tabs.length - 1 }[event.key];
    if (index !== undefined) { event.preventDefault(); selectView(tabs[index].dataset.view, true); }
  };
});
function theme(mode) {
  document.documentElement.dataset.theme = mode;
  const label = mode === 'dark' ? '切换为亮色' : '切换为暗色';
  $('#theme').setAttribute('aria-label', label);
  $('#theme').dataset.tooltip = label;
  $('#theme').replaceChildren(icon(mode === 'dark' ? 'Sun' : 'Moon'));
}
try { theme(localStorage.getItem('theme') === 'light' ? 'light' : 'dark'); } catch { theme('dark'); }
$('#theme').onclick = () => {
  const mode = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
  theme(mode);
  try { localStorage.setItem('theme', mode); } catch {}
};
function openSettings() {
  if (!$('#settingsDialog').open) $('#settingsDialog').showModal();
}
$('#settings').onclick = openSettings;
$('#connectSettings').onclick = openSettings;
$('#closeSettings').onclick = () => $('#settingsDialog').close();
$('#settingsDialog').onclick = event => { if (event.target === $('#settingsDialog')) {
  const rect = event.target.getBoundingClientRect();
  if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) event.target.close();
} };
config.oninput = () => { configDirty = true; };
$('#showKey').onclick = () => {
  const show = config.elements.gateway_key.type === secretType;
  config.elements.gateway_key.type = show ? 'text' : secretType;
  $('#showKey').replaceChildren(icon(show ? 'EyeOff' : 'Eye'));
  $('#showKey').setAttribute('aria-label', show ? '隐藏输入的 Key' : '显示输入的 Key');
  $('#showKey').dataset.tooltip = $('#showKey').getAttribute('aria-label');
};
function requireGateway() {
  if (data?.gateway_configured) return true;
  openSettings();
  $('#configMessage').textContent = '请保存 Magpie 网关 URL 和 Key；本机默认 Key 为 magpie。';
  config.elements.gateway_url.focus();
  return false;
}
function modelRunning(model) { return data?.state.results?.some(r => r.model === model && r.verdict === 'running'); }
function renderControls() {
  if (!data) return;
  $('#run').disabled = $('#run').hasAttribute('aria-busy') || !$('#model').value || modelRunning($('#model').value);
  $('#testStatus').textContent = modelRunning($('#model').value) ? '检测进行中，请等待回答。' : '';
  renderModelTrace(data);
}
function showPrompt() {
  $('#promptText').textContent = data?.prompts.find(p => p.id === $('#prompt').value)?.text || '';
  $('#guidance').textContent = data?.guidance?.[$('#prompt').value] || '';
}
function dayCount(model) {
  const day = new Date().toISOString().slice(0, 10);
  return data.state.results.filter(r => (!model || r.model === model) && new Date(r.time).toISOString().slice(0, 10) === day).length;
}
function validPass(target) { return target.enabled && target.state === 'passed' && Date.now() - Date.parse(target.checked_at) < 900000; }
function renderTargets() {
  const targets = Object.values(data.state.targets);
  const signature = JSON.stringify([targets, data.state.results.filter(r => r.verdict === 'running').map(r => r.model), data.state.config.daily_limit, targets.map(t => [dayCount(t.model), validPass(t), Date.parse(t.allow_until) > Date.now()])]);
  if (signature === targetSignature) return;
  targetSignature = signature;
  const rows = targets.map(target => {
    const row = document.createElement('tr');
    const model = text('td', target.model, 'model-id');
    model.dataset.label = '模型';
    const status = text('td', '');
    status.dataset.label = '状态';
    const allowed = Date.parse(target.allow_until) > Date.now();
    const label = !target.enabled ? '监测关闭' : allowed ? '限时放行' : target.state === 'passed' && !validPass(target) ? '通过已过期' : names[target.state] || target.state;
    status.append(text('span', label, 'state ' + (target.enabled ? target.state : '')));
    if (target.reason) status.append(text('span', target.reason, 'cell-meta'));
    const checked = text('td', dateTime(target.checked_at));
    checked.dataset.label = '最近通过';
    const budget = text('td', `${dayCount(target.model)} / ${data.state.config.daily_limit}`, 'numeric');
    budget.dataset.label = '今日预算 (UTC)';
    const actions = text('td', '');
    actions.dataset.label = '操作';
    const tools = text('div', '', 'actions');
    for (const [name, action, image] of [['重新检测', 'test', 'RefreshCw'], ['放行 15 分钟', 'allow', 'Clock3'], ['暂停', 'hold', 'Pause']]) {
      const button = text('button', '', 'icon-button');
      button.type = 'button'; button.append(icon(image));
      button.setAttribute('aria-label', `${name} ${target.model}`);
      button.dataset.tooltip = name; button.title = name;
      button.disabled = !target.enabled || (action === 'test' && modelRunning(target.model));
      button.onclick = () => pending(button, async () => {
        if (action === 'test' && !requireGateway()) return;
        await request('/api/action', { model: target.model, action });
        feedback(`${target.model}：${action === 'test' ? '已开始检测' : action === 'allow' ? '已放行 15 分钟' : '已暂停'}`, true);
        await refresh();
      });
      tools.append(button);
    }
    const toggle = text('label', '', 'toggle');
    const input = document.createElement('input'); input.type = 'checkbox'; input.checked = target.enabled;
    input.setAttribute('aria-label', `监测 ${target.model}`);
    input.onchange = async () => {
      input.disabled = true;
      try { await request('/api/target', { model: target.model, enabled: input.checked }); await refresh(); }
      catch (error) { input.checked = target.enabled; feedback(error.message); }
      finally { input.disabled = false; }
    };
    toggle.append(input, text('span', '', 'toggle-track')); tools.append(toggle); actions.append(tools);
    row.append(model, status, checked, budget, actions);
    return row;
  });
  if (!rows.length) {
    const row = text('tr', '', 'empty-row'); const cell = text('td', '添加一个模型开始监测', 'empty-state'); cell.colSpan = 5; row.append(cell); rows.push(row);
  }
  $('#targets').replaceChildren(...rows);
}
function renderHistory() {
  if (!data) return;
  const filtered = [...data.state.results].reverse().filter(r => (!$('#historyModel').value || r.model === $('#historyModel').value) && (!$('#historyVerdict').value || r.verdict === $('#historyVerdict').value));
  $('#historyCount').textContent = `${filtered.length} 条记录`;
  $('#loadHistory').hidden = filtered.length <= historyLimit;
  const records = filtered.slice(0, historyLimit);
  const signature = JSON.stringify(records);
  if (signature === historySignature) return;
  historySignature = signature;
  const expanded = new Set([...$('#results').querySelectorAll('details[open]')].map(d => d.dataset.id));
  $('#results').replaceChildren(...records.map(record => {
    const details = text('details', '', 'record'); details.dataset.id = record.id; details.open = expanded.has(record.id);
    const summary = text('summary', '', 'history-summary');
    summary.append(text('span', dateTime(record.time), 'record-date'), text('span', record.model, 'record-model model-id'), text('span', data.prompts.find(p => p.id === record.prompt)?.name || 'ModelTrace', 'record-type'), text('span', names[record.verdict] || record.verdict, 'state ' + record.verdict), text('span', `${record.tokens || 0} tokens`, 'record-tokens numeric'));
    const body = text('div', '', 'record-body');
    body.append(text('pre', record.error || record.answer || '等待回答'));
    const download = text('a', '下载原始回答', 'download'); download.prepend(icon('Download'));
    download.href = '/api/results/' + encodeURIComponent(record.id) + '/download'; body.append(download);
    const prompt = record.prompt_text || data.prompts.find(p => p.id === record.prompt)?.text;
    if (prompt) body.append(text('p', '提示词原文', 'prompt-label'), text('pre', prompt));
    details.append(summary, body); return details;
  }));
  if (!records.length) $('#results').append(text('p', '暂无符合条件的记录', 'empty-state'));
}
async function refresh() {
  if (polling || stopped) return;
  polling = true;
  try {
    data = await request('/api/state');
    if (pollError && $('#message').textContent === pollError) feedback('');
    pollError = '';
    $('#health').textContent = data.health ? '服务异常' : data.gateway_configured ? '网关已配置' : '等待连接';
    $('#health').className = 'service-state ' + (data.health ? 'warning' : data.gateway_configured ? 'connected' : '');
    $('#health').title = data.health || '';
    $('#connectionNotice').hidden = data.gateway_configured;
    $('#keyState').textContent = data.gateway_configured ? 'Key 已保存；留空保留已保存的 Key。' : '尚未保存 Key；本机默认 Key 为 magpie。';
    $('#logHealth').textContent = data.log_health;
    if (!configDirty) for (const key of ['gateway_url', 'log_path', 'daily_limit']) config.elements[key].value = data.state.config[key];
    const targets = Object.values(data.state.targets);
    $('#targetCount').textContent = targets.length;
    $('#statEnabled').textContent = targets.filter(t => t.enabled).length;
    $('#statPassed').textContent = targets.filter(validPass).length;
    $('#statReview').textContent = targets.filter(t => t.enabled && ['review', 'unavailable'].includes(t.state)).length;
    $('#statRequests').textContent = dayCount();
    syncSelect($('#model'), targets.filter(t => t.enabled).map(t => [t.model, t.model]));
    syncSelect($('#prompt'), data.prompts.map(p => [p.id, p.name]));
    syncSelect($('#historyModel'), [['', '全部模型'], ...[...new Set([...targets.map(t => t.model), ...data.state.results.map(r => r.model)])].map(m => [m, m])]);
    showPrompt(); renderTargets(); renderControls(); renderHistory();
    $('#lastSync').textContent = '更新于 ' + dateTime(new Date().toISOString());
  } catch (error) {
    pollError = error.message; feedback(pollError);
    $('#health').textContent = '连接中断'; $('#health').className = 'service-state warning';
  } finally { polling = false; }
}
$('#prompt').onchange = showPrompt;
$('#model').onchange = renderControls;
$('#historyModel').onchange = $('#historyVerdict').onchange = () => { historyLimit = 30; renderHistory(); };
$('#loadHistory').onclick = () => { historyLimit += 30; renderHistory(); };
$('#run').onclick = () => pending($('#run'), async () => {
  if (!requireGateway()) return;
  await request('/api/action', { model: $('#model').value, action: 'test', prompt: $('#prompt').value });
  feedback('已开始检测', true); await refresh();
});
$('#traceRun').onclick = () => pending($('#traceRun'), async () => {
  if (!requireGateway()) return;
  await runModelTrace(request); feedback('已开始三条指纹挑战', true); await refresh();
});
$('#loadModels').onclick = () => pending($('#loadModels'), async () => {
  if (!requireGateway()) return;
  const models = await request('/api/models');
  $('#catalog').replaceChildren(...models.map(m => { const option = text('option', m.group); option.value = m.id; return option; }));
  feedback(`已读取 ${models.length} 个模型`, true);
});
$('#target').onsubmit = event => {
  event.preventDefault();
  const form = event.target;
  pending(form.querySelector('button:last-child'), async () => {
    await request('/api/target', { model: form.elements.model.value.trim(), enabled: true });
    form.reset(); feedback('已添加监测目标', true); await refresh();
  });
};
async function saveConfig() {
  const values = Object.fromEntries(new FormData(config)); values.daily_limit = Number(values.daily_limit);
  await request('/api/config', values);
  config.elements.gateway_key.value = ''; config.elements.gateway_key.type = secretType;
  $('#showKey').replaceChildren(icon('Eye'));
  $('#showKey').setAttribute('aria-label', '显示输入的 Key');
  $('#showKey').dataset.tooltip = '显示输入的 Key';
  configDirty = false; await refresh();
}
async function checkConnection() {
  const result = await request('/api/connection/check', {});
  $('#configMessage').textContent = `连接成功：${result.model_count} 个模型`;
  $('#configMessage').className = 'inline-state success';
}
config.onsubmit = event => {
  event.preventDefault();
  pending(config.querySelector('button.primary'), async () => {
    await saveConfig();
    $('#configMessage').textContent = '设置已保存，正在验证连接…';
    await checkConnection();
  }, $('#configMessage'));
};
$('#checkConnection').onclick = () => pending($('#checkConnection'), async () => {
  if (configDirty || !data?.gateway_configured) {
    $('#configMessage').textContent = '请先保存当前连接设置。'; return;
  }
  await checkConnection();
}, $('#configMessage'));
$('#closeRemote').onclick = () => pending($('#closeRemote'), async () => {
  await request('/api/console/close', {}); stopped = true;
  feedback('远程访问已结束，请从插件重新打开。');
});
await refresh();
setInterval(refresh, 3000);
