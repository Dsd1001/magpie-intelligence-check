const $ = s => document.querySelector(s);
const panel = $('#targets');
const history = $('#results');
const notice = $('#notice');

async function request(path, body) {
  const res = await fetch(path, {method:body ? 'POST' : 'GET',
    headers:body ? {'Content-Type':'application/json'} : {},
    body:body ? JSON.stringify(body) : undefined});
  if (!res.ok) throw Error(res.status === 401 ? '管理会话过期，请从 Magpie 重新打开插件控制台' : await res.text());
  return res.json();
}
function td(tr, val, cls) {
  const cell = document.createElement('td');
  cell.textContent = val ?? '';
  if (cls) cell.className = cls;
  tr.append(cell);
  return cell;
}
function stamp(date) { return date && date !== '0001-01-01T00:00:00Z' ? new Date(date).toLocaleString() : '—'; }
function button(parent, label, payload, url) {
  const el=document.createElement('button'); el.textContent=label;
  el.onclick=async()=>{el.disabled=true; try{await request(url,payload);notice.textContent='已提交';await refresh();}
   catch(e){notice.textContent=e.message;}finally{el.disabled=false;}};
  parent.append(el);
}
async function refresh() {
 try {
  const data=await request('/api/account-state');
  const targets=Object.values(data.targets||{});
  panel.replaceChildren();
  if (!targets.length) {
    const r=document.createElement('tr');td(r,'尚未添加检测目标').colSpan=5;panel.append(r);
  }
  for (const t of targets) {
    const tr=document.createElement('tr');
    td(tr,t.account);td(tr,t.model);
    td(tr, (t.state||'unverified')+(t.reason ? '：'+t.reason : ''),t.state==='passed'?'pass':t.state==='suspect'?'failed':'');
    td(tr,stamp(t.checked_at));
    const actions=td(tr,'');actions.className='actions';
    button(actions,t.enabled?'开始检测':'启用',{account:t.account,model:t.model,...(t.enabled?{prompt:'milk'}:{enabled:true})},t.enabled?'/api/account-test':'/api/account-target');
    button(actions,t.enabled?'禁用':'删除',{account:t.account,model:t.model,enabled:false},t.enabled?'/api/account-target':'/api/account-target/delete');
    panel.append(tr);
  }
  history.replaceChildren();
  for(const entry of (data.results||[]).slice(-40).reverse()){
    const tr=document.createElement('tr');
    td(tr,stamp(entry.time));
    td(tr,entry.account+' / '+entry.model);
    td(tr,entry.verdict+(entry.error?' ('+entry.error+')':''),entry.verdict==='passed'?'pass':entry.verdict==='failed'?'failed':'');
    td(tr,String(entry.tokens??0));
    history.append(tr);
  }
 } catch(e){notice.textContent=e.message;}
}
$('#add').onsubmit=async event=>{
 event.preventDefault();const form=event.currentTarget,fields=new FormData(form);
 try{await request('/api/account-target',{account:String(fields.get('account')).trim(),model:String(fields.get('model')).trim(),enabled:true}); form.reset();notice.textContent='已添加';await refresh();}
 catch(e){notice.textContent=e.message;}
};
$('#refresh').onclick=refresh;
refresh();
setInterval(refresh,10000);

// The native Magpie account ID, not a stored OAuth token, selects an account.
async function loadAccountSuggestions(){
  try {
    const data=await request('/api/codex-accounts');
    const list=$('#knownAccounts');
    list.replaceChildren();
    for(const a of (data.accounts||[])){
      const item=document.createElement('option');
      item.value=a.user;
      item.label=a.id;
      list.append(item);
    }
  }catch(e){notice.textContent='账号发现不可用：'+e.message;}
}
async function loadRoutingSettings(){
  try{
    const data=await request('/api/routing/settings');
    const route=data.route||{};
    const form=$('#routingForm');
    form.elements.probe_key_id.value=route.probe_key_id||'';
    form.elements.expected_codex_count.value=route.expected_codex_count||'';
    form.elements.auto_apply.checked=Boolean(route.auto_apply);
    form.elements.schedule_enabled.checked=Boolean(route.schedule_enabled);
    $('#routingStatus').textContent=route.pending?'存在待核对的未完成路由操作':(route.last_error||'路由控制器：'+(route.auto_apply?'已授权自动同步':'只观察'));
  }catch(e){$('#routingStatus').textContent=e.message;}
}
async function previewRouting(){
  const preview=$('#routingPreview');
  try{
    const result=await request('/api/routing/preview');
    const rows=[
      '模式：'+(result.mode==='active'?'已启用自动隔离':'只观察'),
      '已发现 Codex 账号：'+result.account_count,
      '隔离 ID：'+((result.quarantined||[]).join(', ')||'无'),
      '将改变的生产 Key：'+(result.keys||[]).filter(k=>k.changed).length,
    ];
    for (const key of result.keys||[]) {
      rows.push((key.is_probe?'检测 Key ':'生产 Key ')+key.key_id+': '+
        (key.is_probe?'始终不变':key.changed?'需要同步':'无需更改')+
        '；原权限 ['+(key.before||[]).join(', ')+'] → 目标 ['+(key.after||[]).join(', ')+']');
    }
    preview.textContent=rows.join('\n');
  }catch(e){preview.textContent='安全检查未通过：'+e.message;}
}
$('#routingForm').onsubmit=async (event)=>{
  event.preventDefault();
  const form=event.currentTarget;
  try{
    await request('/api/routing/config',{
      probe_key_id:form.elements.probe_key_id.value.trim(),
      expected_codex_count:Number(form.elements.expected_codex_count.value||0),
      schedule_enabled:form.elements.schedule_enabled.checked,
      auto_apply:form.elements.auto_apply.checked,
      confirm:form.elements.confirm.value.trim()
    });
    form.elements.confirm.value='';
    $('#routingStatus').textContent='保存成功。启用后在下一轮调度或手动同步时才会写入白名单。';
    await loadRoutingSettings(); await previewRouting();
  }catch(e){$('#routingStatus').textContent=e.message;}
};
$('#previewRoutes').onclick=previewRouting;
for(const [id,path,confirm] of [
  ['applyRoutes','/api/routing/apply','APPLY_QUARANTINE'],
  ['restoreRoutes','/api/routing/restore','RESTORE_ALL_PRODUCTION_KEYS'],
  ['recoverRoutes','/api/routing/recover','RECOVER_PENDING_JOURNAL']
]){
  $('#'+id).onclick=async()=>{
    if($('#routingActionConfirm').value.trim()!==confirm){
      $('#routingStatus').textContent='请输入正确的操作确认短语：'+confirm;return;
    }
    try{
      await request(path,{confirm});
      $('#routingActionConfirm').value='';
      $('#routingStatus').textContent='操作成功，已记录审计日志。';
      await loadRoutingSettings();await previewRouting();
    }catch(e){$('#routingStatus').textContent=e.message;}
  };
}
loadAccountSuggestions();
loadRoutingSettings();

$('#discoverAll').onclick=async()=>{
  const model=document.querySelector('#add').elements.model.value.trim();
  if (!model) {notice.textContent='请先输入完整模型 ID';return;}
  try {
    const r=await request('/api/account-targets/discover',{model,confirm:'ADD_ALL_CODEX_ACCOUNTS'});
    notice.textContent='已匹配 '+r.count+' 个 Magpie Codex 账号，现可逐一检测。';
    await refresh();
  }catch(e){notice.textContent=e.message;}
};
