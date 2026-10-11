import { analyzeGlobalOutputs } from './fingerprint-core.js';

let bank, bankLoading, latestData, rendered;
const $ = selector => document.querySelector(selector);
const text = (tag, value) => {
  const node = document.createElement(tag);
  node.textContent = value;
  return node;
};
const percent = value => `${(value * 100).toFixed(1)}%`;

export async function runModelTrace(request) {
  const model = $('#traceModel').value;
  if (!model) throw Error('请先添加监测模型。');
  await request('/api/action', { model, action: 'modeltrace' });
}

export function renderModelTrace(data) {
  latestData = data;
  const select = $('#traceModel');
  const selected = select.value;
  select.replaceChildren();
  for (const target of Object.values(data.state.targets)) {
    if (!target.enabled) continue;
    const option = text('option', target.model);
    option.value = target.model;
    select.append(option);
  }
  if ([...select.options].some(option => option.value === selected)) select.value = selected;
  select.onchange = () => { rendered = undefined; renderModelTrace(latestData); };
  const records = data.state.results.filter(r => r.prompt === 'modeltrace' && r.model === select.value);
  const batch = records.at(-1)?.batch;
  const results = records.filter(r => r.batch === batch);
  $('#traceRun').disabled = !select.value || results.some(r => r.verdict === 'running');
  const signature = JSON.stringify([select.value, results, Boolean(bank)]);
  if (signature === rendered) return;
  rendered = signature;
  const panel = $('#traceResults');
  panel.replaceChildren();
  if (!batch) {
    panel.append(text('p', '暂无指纹检测记录'));
    return;
  }
  panel.append(text('p', `${select.value} / ${new Date(results[0].time).toLocaleString()} / ${results.filter(r => r.verdict !== 'running').length}/3 完成`));
  if (results.some(r => r.verdict === 'running')) return;
  if (results.length !== 3 || results.some(r => r.verdict !== 'manual')) {
    panel.append(text('p', '本轮检测不可用：' + (results.find(r => r.error)?.error || '缺少三条完整回答')));
    return;
  }
  if (!bank) {
    panel.append(text('p', '正在读取指纹库'));
    if (!bankLoading) {
      bankLoading = fetch('/modeltrace/unified_bank.json').then(async response => {
        if (!response.ok) throw Error(`指纹库读取失败（HTTP ${response.status}），请重新打开控制台。`);
        bank = await response.json();
        rendered = undefined;
        renderModelTrace(latestData);
      }).catch(error => {
        panel.replaceChildren(text('p', error.message));
        rendered = undefined;
      }).finally(() => { bankLoading = undefined; });
    }
    return;
  }
  try {
    const report = analyzeGlobalOutputs(results.map(r => ({ text: r.answer, expected_count: r.expected_count })), bank);
    if (report.used_outputs !== 3) {
      panel.append(text('p', `仅 ${report.used_outputs}/3 条有效数字回答，本轮不作归因。`));
      return;
    }
    panel.append(text('p', `最相似候选：${report.prediction_name} / ${percent(report.probability)}；家族：${report.family_prediction_name} / ${percent(report.family_probability)}`));
    const details = document.createElement('details');
    details.append(text('summary', `全部 ${report.results.length} 个候选`));
    const scroll = document.createElement('div');
    scroll.className = 'scroll';
    const table = document.createElement('table');
    const header = document.createElement('tr');
    header.append(text('th', '候选模型'), text('th', '家族'), text('th', '闭集概率'));
    const head = document.createElement('thead');
    head.append(header);
    table.append(head);
    const body = document.createElement('tbody');
    for (const candidate of report.results) {
      const row = document.createElement('tr');
      row.append(text('td', candidate.display_name), text('td', candidate.family_name), text('td', percent(candidate.probability)));
      body.append(row);
    }
    table.append(body);
    scroll.append(table);
    details.append(scroll);
    panel.append(details);
  } catch (error) {
    panel.append(text('p', `本轮无法归因：${error.message}`));
  }
}
