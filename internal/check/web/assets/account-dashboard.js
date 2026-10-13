import { $, text, syncSelect, dateTime } from './ui.js';

// This module is part of the existing Intelligence Check dashboard.
// Only login EMAILS are rendered. Account stable IDs remain private values
// used for API lookups, so routing isolation survives account renames.
export function initAccountViews({ request, onSummaryChange }) {
  const state = {
    active: 'models',
    accounts: [],
    models: [],
    prompts: [],
    autoBank: { prompt_ids: ['milk'], rotate: true },
    autoBankDirty: false,
    keys: [],
    targets: [],
    results: [],
    route: {},
    loaded: false,
    loading: false,
    routingDirty: false,
  };
  const routeForm = $('#routingForm');
  const accountForm = $('#accountAddForm');

  function notice(selector, message, error = false) {
    const el = $(selector);
    el.textContent = message || '';
    el.className = 'inline-state' + (error ? ' error' : message ? ' success' : '');
  }

  async function perform(button, work, target = '#accountNotice') {
    if (button.disabled) return;
    button.disabled = true;
    button.setAttribute('aria-busy', 'true');
    try {
      await work();
    } catch (error) {
      notice(target, error.message, true);
    } finally {
      button.disabled = false;
      button.removeAttribute('aria-busy');
    }
  }

  // Stable ID can be in a target or a result, but the browser displays
  // only the matching human-readable login. Missing references are hidden.
  function accountIdentity(ref) {
    return state.accounts.find(a =>
      String(a.id).toLowerCase() === String(ref).toLowerCase() ||
      String(a.user).toLowerCase() === String(ref).toLowerCase());
  }
  function email(ref) {
    const matched = accountIdentity(ref);
    if (matched) return matched.user;
    return String(ref || '').includes('@') ? String(ref) : '账号已移除';
  }
  function stableID(ref) {
    return accountIdentity(ref)?.id || ref;
  }
  function displayError(value) {
    let message = String(value || '');
    for (const account of state.accounts) {
      if (account.id) message = message.replaceAll(account.id, account.user);
    }
    // Old errors may contain IDs for accounts no longer saved in Magpie.
    return message.replace(/\b[a-f0-9]{16}\b/gi, '已移除账号');
  }
  function time(value) { return Date.parse(value) > 0 ? dateTime(value) : '—'; }
  function rowCell(row, label, value, className = '') {
    const td = text('td', value, className);
    td.dataset.label = label;
    row.append(td);
    return td;
  }

  function status(target) {
    const isolated = Boolean(state.route.quarantined?.[stableID(target.account)]);
    if (isolated) return ['已隔离', 'isolated'];
    switch (target.state) {
      case 'passed': return ['通过', 'passed'];
      case 'suspect': return ['疑似异常', 'suspect'];
      case 'checking': return ['检测中', 'checking'];
      case 'unavailable': return ['检测不可用', 'unavailable'];
      case 'manual': return ['待人工评价', 'review'];
      default: return ['未检测', 'unverified'];
    }
  }
  function labelState(target) {
    const [label, style] = status(target);
    const wrap = text('span', label, 'state ' + style);
    return wrap;
  }
  function action(label, click, cls = 'secondary', disabled = false) {
    const b = text('button', label, cls);
    b.type = 'button';
    b.disabled = disabled;
    b.onclick = click;
    return b;
  }
  function empty(table, colspan, message) {
    const tr = document.createElement('tr');
    const td = rowCell(tr, '', message, 'empty-state');
    td.colSpan = colspan;
    table.append(tr);
  }

  function refreshSelectors() {
    const allAccounts = [['', '选择账号邮箱'], ...state.accounts.map(a => [a.id, a.user])];
    syncSelect($('#accountSelect'), allAccounts);
    const modelOptions = [['', state.models.length ? '选择模型' : '请先连接网关并刷新模型'],
      ...state.models.map(m => [m.id, m.id])];
    syncSelect($('#accountModel'), modelOptions);
    syncSelect($('#accountPrompt'), state.prompts.length
      ? state.prompts.map(p => [p.id, p.name + (p.automatic ? ' · 自动判分' : ' · 人工评价')])
      : [['milk', '牛奶推理 · 自动判分']]);
    const historyList = [['', '全部账号'], ...state.accounts.map(a => [a.id, a.user])];
    syncSelect($('#accountHistoryFilter'), historyList);

    const keyOptions = [['', '选择检测专用 Key'],
      ...state.keys.map(k => [k.id, (k.name || '未命名 Gateway Key') + (k.off ? '（已禁用）' : '')])];
    syncSelect($('#routingProbeKey'), keyOptions);
    const saved = state.route.probe_key_id;
    if (saved && !keyOptions.some(([id]) => id === saved)) {
      const option = text('option', '已保存的检测 Key 不存在，请重新选择');
      option.value = saved;
      $('#routingProbeKey').append(option);
    }
  }

  function chosenPrompt() { return $('#accountPrompt').value || 'milk'; }
  function promptName(id) { return state.prompts.find(p => p.id === id)?.name || id || '题目已移除'; }
  function promptHint() {
    const selected = state.prompts.find(p => p.id === chosenPrompt());
    $('#accountPromptHint').textContent = selected && !selected.automatic
      ? '「' + selected.name + '」仅用于单次手动检测，回答可在历史中查看，不计入自动隔离。'
      : '该题目支持自动判分。启用自动检测的题目与牛奶题按小时轮换；单题首次错误后保持同题每 2 分钟复测，连续 3 次才可能触发隔离。';
  }
  function renderAutoBank() {
    if (state.autoBankDirty) return;
    const eligible = state.prompts.filter(p => p.automatic);
    const chosen = new Set(state.autoBank.prompt_ids || []);
    const list = $('#autoBankOptions');
    list.replaceChildren();
    for (const prompt of eligible) {
      const label = text('label', '', 'auto-bank-item');
      const check = document.createElement('input');
      check.type = 'checkbox'; check.value = prompt.id; check.checked = chosen.has(prompt.id);
      check.onchange = () => { state.autoBankDirty = true; updateAutoBankCount(); };
      label.append(check, text('span', prompt.name));
      list.append(label);
    }
    $('#autoBankRotate').checked = Boolean(state.autoBank.rotate);
    $('#autoBankCount').textContent = String(chosen.size);
    $('#autoBankStatus').textContent = state.autoBank.configured ? '已保存' : '默认：全部自动题';
  }
  function updateAutoBankCount() {
    $('#autoBankCount').textContent = String($('#autoBankOptions').querySelectorAll('input:checked').length);
  }
  $('#autoBankDropdown').onclick = () => {
    const el = $('#autoBankOptions');
    el.hidden = !el.hidden;
    $('#autoBankDropdown').setAttribute('aria-expanded', String(!el.hidden));
  };
  $('#autoBankRotate').onchange = () => { state.autoBankDirty = true; };
  $('#autoBankSave').onclick = () => perform($('#autoBankSave'), async () => {
    const prompt_ids = [...$('#autoBankOptions').querySelectorAll('input:checked')].map(x=>x.value);
    if (!prompt_ids.length) throw Error('请至少选择一道自动检测题目');
    const result = await request('/api/account-auto-prompts', {
      prompt_ids, rotate: $('#autoBankRotate').checked,
    });
    state.autoBank = result;
    state.autoBankDirty = false;
    $('#autoBankOptions').hidden = true;
    $('#autoBankDropdown').setAttribute('aria-expanded','false');
    renderAutoBank();
    notice('#autoBankNotice', '自动题库已保存。' +
      (result.rotate ? '已开启轮换。' : '已关闭轮换，固定使用第一道选中题。'));
  }, '#autoBankNotice');

  function renderTargets() {
    const rows = [...state.targets].sort((a, b) =>
      (email(a.account) + '/' + a.model).localeCompare(email(b.account) + '/' + b.model, 'zh-CN'));
    const enabled = rows.filter(t => t.enabled).length;
    const isolated = rows.filter(t => t.enabled && state.route.quarantined?.[stableID(t.account)]).length;
    $('#accountSummary').textContent = `${enabled} 个监测目标 · ${isolated} 个隔离中`;
    $('#accountTabCount').textContent = enabled;
    $('#accountTargetCount').textContent = `${rows.length} 条监测配置`;
    const table = $('#accountTargets');
    table.replaceChildren();
    if (!rows.length) {
      empty(table, 5, '尚无检测目标。选择模型和账号后添加，或一键添加所有账号。');
      return;
    }
    for (const t of rows) {
      const tr = document.createElement('tr');
      rowCell(tr, '账号邮箱', email(t.account), 'email');
      rowCell(tr, '模型', t.model, 'model-id');
      const cell = rowCell(tr, '检测状态', '');
      cell.append(labelState(t));
      const detail = t.state === 'suspect'
        ? `连续 ${t.consecutive_fails || 0} 次异常`
        : t.state === 'passed'
          ? `连续 ${t.consecutive_passes || 0} 次通过`
          : t.state === 'unavailable'
            ? '调用不可用，不计入质量失败'
            : !t.enabled ? '监测已暂停' : '';
      if (!t.enabled) {
        cell.replaceChildren(text('span', '监测已暂停', 'state muted'));
      }
      if (detail) cell.append(text('span', detail, 'cell-meta'));
      const when = rowCell(tr, '上次检测', time(t.checked_at), 'numeric');
      if (t.reason) {
        const desc = displayError(t.reason);
        // Do not surface a stable ID in an old error.
        when.title = desc;
      }
      const actions = rowCell(tr, '操作', '', 'align-right');
      const group = text('div', '', 'actions');
      group.append(
        action('检测', () => perform(group.children[0], async () => {
          const prompt = chosenPrompt();
          await request('/api/account-test', { account: t.account, model: t.model, prompt });
          notice('#accountNotice', '已开始对 ' + email(t.account) + ' 运行「' + promptName(prompt) + '」');
          await refreshState();
        }), 'secondary', !t.enabled || t.state === 'checking'),
        action(t.enabled ? '暂停' : '启用', () => perform(group.children[1], async () => {
          await request('/api/account-target', { account: t.account, model: t.model, enabled: !t.enabled });
          await refreshState();
        }), 'secondary', t.state === 'checking'),
        action('移除', () => perform(group.children[2], async () => {
          if (!confirm('移除 ' + email(t.account) + ' 的 ' + t.model + ' 监测目标？历史结果会保留。')) return;
          await request('/api/account-target/delete', { account: t.account, model: t.model });
          await refreshState();
        }), 'text-button remove-action', t.state === 'checking')
      );
      actions.append(group);
      table.append(tr);
    }
  }

  function renderHistory() {
    const tbody = $('#accountHistory');
    tbody.replaceChildren();
    const selected = $('#accountHistoryFilter').value;
    const records = state.results.filter(r => !selected || stableID(r.account) === selected).slice(-35).reverse();
    if (!records.length) {
      empty(tbody, 6, '暂无检测记录');
      return;
    }
    const statuses = {
      passed: ['通过', 'passed'], failed: ['回答错误', 'failed'],
      unavailable: ['不可用', 'unavailable'], manual: ['人工评价', 'review'],
    };
    for (const r of records) {
      const tr = document.createElement('tr');
      rowCell(tr, '检测时间', time(r.time), 'numeric');
      rowCell(tr, '账号邮箱', email(r.account), 'email');
      rowCell(tr, '模型', r.model, 'model-id');
      rowCell(tr, '检测题目', r.prompt_name || promptName(r.prompt));
      const [value, style] = statuses[r.verdict] || ['未确认', 'unverified'];
      const verdict = rowCell(tr, '检测结果', '');
      verdict.append(text('span', value, 'state ' + style));
      if (r.answer || r.error) {
        const button = text('button', '查看回答', 'text-button account-show-answer');
        button.type = 'button';
        button.onclick = () => showAnswer(r);
        verdict.append(button);
      }
      rowCell(tr, 'Token', String(r.tokens || 0), 'numeric');
      if (r.error) tr.title = displayError(r.error);
      tbody.append(tr);
    }
  }

  function showAnswer(record) {
    const dialog = $('#accountAnswerDialog');
    $('#accountAnswerTitle').textContent = record.prompt_name || promptName(record.prompt);
    $('#accountAnswerMeta').textContent =
      email(record.account) + ' · ' + record.model + ' · ' + time(record.time);
    $('#accountAnswerText').textContent = record.answer ||
      (record.error ? displayError(record.error) : '本次请求没有返回回答正文');
    dialog.showModal();
  }

  function updateRouteForm() {
    if (state.routingDirty) return; // never overwrite unsaved switches or typing
    const route = state.route;
    routeForm.elements.probe_key_id.value = route.probe_key_id || '';
    routeForm.elements.expected_codex_count.value =
      route.expected_codex_count || state.accounts.length || '';
    routeForm.elements.schedule_enabled.checked = Boolean(route.schedule_enabled);
    routeForm.elements.auto_apply.checked = Boolean(route.auto_apply);
    routeForm.elements.confirm.value = '';
    $('#routingEnableConfirm').hidden = !routeForm.elements.auto_apply.checked;
    $('#routingMode').textContent = route.pending ? '需要人工核对' :
      route.auto_apply ? '自动隔离已启用' : route.schedule_enabled ? '仅观察 · 定时检测中' : '未启用';
    $('#routingMode').className = 'state ' +
      (route.pending ? 'unavailable' : route.auto_apply ? 'passed' : 'muted');
    $('#discoveredAccountCount').textContent = `当前识别 ${state.accounts.length} 个 Codex 账号`;
    if (route.last_error) {
      notice('#routingNotice', displayError(route.last_error), true);
    }
  }

  function displayKeyRef(ref) {
    if (String(ref).startsWith('codex/')) return email(String(ref).slice(6));
    return String(ref || '').replace(/\b[a-f0-9]{16}\b/gi, '已移除账号');
  }
  function displayScope(refs) {
    return !refs?.length ? '全部账号（无限制）' : refs.map(displayKeyRef).join(' · ');
  }
  function renderPreview(preview) {
    const container = $('#routingPreview');
    container.replaceChildren();
    const keys = preview.keys || [], quarantined = preview.quarantined || [];
    $('#routingPreviewStatus').textContent =
      `${keys.filter(k => k.changed).length} 个 Key 需要同步`;
    const summary = text('div', '', 'routing-summary');
    const parts = [
      `检测账号 ${preview.account_count} 个`,
      `隔离账号：${quarantined.length ? quarantined.map(email).join('、') : '无'}`,
      preview.mode === 'active' ? '自动隔离：已启用' : '自动隔离：未启用',
    ];
    for (const part of parts) summary.append(text('span', part));
    container.append(summary);
    for (const key of keys) {
      const holder = text('div', '', 'routing-key');
      const head = text('div', '', 'routing-key-head');
      const keyName = state.keys.find(k => k.id === key.key_id)?.name || 'Gateway Key';
      head.append(
        text('strong', keyName),
        text('span', key.is_probe ? '检测专用 · 始终不变' : key.changed ? '需要同步' : '权限保持不变',
          'key-status' + (key.is_probe ? ' probe' : key.changed ? ' changed' : '')),
      );
      holder.append(head);
      if (!key.is_probe && key.changed) {
        const scopes = text('div', '', 'key-scopes');
        const before = text('div');
        before.append(text('span', '当前权限'), text('strong', displayScope(key.before)));
        const after = text('div');
        after.append(text('span', '隔离后权限'), text('strong', displayScope(key.after)));
        scopes.append(before, text('span', '→'), after);
        holder.append(scopes);
      } else if (!key.is_probe) {
        holder.append(text('span', '账号权限无需更改', 'cell-meta'));
      }
      container.append(holder);
    }
  }

  async function loadAccountsAndKeys() {
    const accounts = await request('/api/codex-accounts');
    state.accounts = [...(accounts.accounts || [])].sort((a, b) =>
      a.user.localeCompare(b.user, 'zh-CN'));
    // A key-metadata read failure must not prevent account quality probes.
    try {
      const keys = await request('/api/routing/keys');
      state.keys = keys.keys || [];
    } catch (error) {
      state.keys = [];
      if (state.active === 'routing') notice('#routingNotice', error.message, true);
    }
    refreshSelectors();
  }

  async function refreshState() {
    if (state.loading) return;
    state.loading = true;
    try {
      const [accountState, routing] = await Promise.all([
        request('/api/account-state'),
        request('/api/routing/settings'),
      ]);
      state.targets = Object.values(accountState.targets || {});
      state.results = accountState.results || [];
      state.route = routing.route || {};
      state.autoBank = accountState.auto_prompt_policy || state.autoBank;
      state.loaded = true;
      refreshSelectors();
      renderAutoBank();
      updateRouteForm();
      renderTargets();
      renderHistory();
      onSummaryChange?.();
    } finally {
      state.loading = false;
    }
  }

  async function reload() {
    try {
      await loadAccountsAndKeys();
      await refreshState();
    } catch (error) {
      const noticeTarget = state.active === 'routing' ? '#routingNotice' : '#accountNotice';
      notice(noticeTarget, error.message, true);
    }
  }

  // The parent dashboard owns the model catalog, so all selectors show
  // the same models and update together after a gateway refresh.
  function setModels(models) {
    state.models = [...(models || [])].sort((a, b) => a.id.localeCompare(b.id));
    refreshSelectors();
  }
  function setPrompts(prompts) {
    state.prompts = prompts || [];
    refreshSelectors();
    renderAutoBank();
    promptHint();
    renderHistory();
  }
  $('#accountPrompt').onchange = promptHint;
  $('#accountAnswerClose').onclick = () => $('#accountAnswerDialog').close();
  $('#accountAnswerDialog').onclick = event => {
    if (event.target === $('#accountAnswerDialog')) event.target.close();
  };
  $('#openCustomPrompts').onclick = () => {
    window.location.hash = '#tests';
    $('#customPromptManager').open = true;
    $('#customPromptForm').elements.name.focus();
  };

  accountForm.onsubmit = event => {
    event.preventDefault();
    perform(accountForm.querySelector('button[type=submit]'), async () => {
      const account = $('#accountSelect').value, model = $('#accountModel').value;
      if (!account || !model) throw Error('请先选择账号邮箱与检测模型');
      await request('/api/account-target', { account, model, enabled: true });
      notice('#accountNotice', '已添加 ' + email(account) + ' 的监测目标');
      await refreshState();
    });
  };

  $('#accountAddAll').onclick = () => perform($('#accountAddAll'), async () => {
    const model = $('#accountModel').value;
    if (!model) throw Error('请先选择模型，再批量添加账号');
    const result = await request('/api/account-targets/discover', {
      model, confirm: 'ADD_ALL_CODEX_ACCOUNTS',
    });
    notice('#accountNotice', `已添加/匹配 ${result.count} 个账号的监测目标`);
    await refreshState();
  });

  $('#accountRefresh').onclick = () => perform($('#accountRefresh'), async () => {
    await reload();
    notice('#accountNotice', '账号与检测记录已刷新');
  });
  $('#accountHistoryFilter').onchange = renderHistory;

  routeForm.oninput = routeForm.onchange = () => {
    state.routingDirty = true;
    $('#routingEnableConfirm').hidden = !routeForm.elements.auto_apply.checked;
  };
  routeForm.onsubmit = event => {
    event.preventDefault();
    perform($('#routingSave'), async () => {
      const auto = routeForm.elements.auto_apply.checked;
      const values = {
        probe_key_id: $('#routingProbeKey').value,
        expected_codex_count: Number(routeForm.elements.expected_codex_count.value || 0),
        schedule_enabled: routeForm.elements.schedule_enabled.checked,
        auto_apply: auto,
        confirm: routeForm.elements.confirm.value.trim(),
      };
      if (!values.probe_key_id) throw Error('请选择检测专用 Gateway Key');
      if (auto && values.confirm !== 'ENABLE_ALL_PRODUCTION_KEYS') {
        throw Error('开启自动隔离需要输入确认短语 ENABLE_ALL_PRODUCTION_KEYS');
      }
      await request('/api/routing/config', values);
      state.routingDirty = false;
      notice('#routingNotice', '设置已保存。生产 Key 的实际同步由调度器执行。');
      await refreshState();
      await previewRoutes();
    }, '#routingNotice');
  };

  async function previewRoutes() {
    const result = await request('/api/routing/preview');
    renderPreview(result);
  }
  $('#routingPreviewButton').onclick = () => perform($('#routingPreviewButton'), async () => {
    await previewRoutes();
    notice('#routingNotice', '已生成路由预览');
  }, '#routingNotice');

  for (const [id, path, required, message] of [
    ['routingApply', '/api/routing/apply', 'APPLY_QUARANTINE', '已同步生产 Key 的白名单'],
    ['routingRestore', '/api/routing/restore', 'RESTORE_ALL_PRODUCTION_KEYS', '已恢复原白名单并关闭自动隔离'],
    ['routingRecover', '/api/routing/recover', 'RECOVER_PENDING_JOURNAL', '已检查未完成的路由事务'],
  ]) {
    const button = $('#' + id);
    button.onclick = () => perform(button, async () => {
      const given = $('#routingActionConfirm').value.trim();
      if (given !== required) throw Error('请先输入对应操作的确认短语：' + required);
      await request(path, { confirm: required });
      $('#routingActionConfirm').value = '';
      state.routingDirty = false;
      notice('#routingNotice', message);
      await refreshState();
      await previewRoutes();
    }, '#routingNotice');
  }

  function activate(view) {
    state.active = view;
    if (view !== 'accounts' && view !== 'routing') return;
    if (!state.loaded) {
      void reload();
    } else {
      void refreshState().catch(error =>
        notice(view === 'routing' ? '#routingNotice' : '#accountNotice', error.message, true));
    }
  }

  // Aggregate account-level checks into the host dashboard's overview;
  // the old model-level counters alone misleadingly showed zero.
  function statistics() {
    const enabled = state.targets.filter(t => t.enabled);
    const today = new Date().toISOString().slice(0, 10);
    return {
      enabled: enabled.length,
      passed: enabled.filter(t => t.state === 'passed').length,
      review: enabled.filter(t => ['suspect', 'unavailable', 'manual'].includes(t.state)).length,
      today: state.results.filter(r => r.time && r.time.slice(0, 10) === today).length,
    };
  }

  // Don't poll the hidden tabs; do update them when the user returns.
  setInterval(() => {
    if (state.active === 'accounts' || state.active === 'routing') {
      void refreshState().catch(error =>
        notice(state.active === 'routing' ? '#routingNotice' : '#accountNotice', error.message, true));
    }
  }, 10000);

  return { activate, reload, setModels, setPrompts, statistics };
}
