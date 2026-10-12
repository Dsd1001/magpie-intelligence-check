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
