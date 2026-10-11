let currentRecord;
const panel = document.querySelector('#htmlPreview');
const host = document.querySelector('#htmlFrameHost');
const status = document.querySelector('#htmlPreviewStatus');
const stop = document.querySelector('#stopHTML');
const reload = document.querySelector('#reloadHTML');
function startPreview() {
  host.replaceChildren();
  if (!currentRecord) return;
  const frame = document.createElement('iframe');
  frame.title = '模型生成的 HTML 实时预览';
  frame.setAttribute('sandbox', 'allow-scripts');
  frame.referrerPolicy = 'no-referrer';
  frame.src = '/api/results/' + encodeURIComponent(currentRecord.id) + '/preview';
  status.textContent = '正在加载…';
  frame.onload = () => { status.textContent = '预览运行中'; };
  host.append(frame);
  stop.disabled = false;
}
stop.onclick = () => {
  host.replaceChildren();
  status.textContent = '预览已停止';
  stop.disabled = true;
};
reload.onclick = startPreview;
export function renderHTMLPreview(record) {
  currentRecord = undefined;
  host.replaceChildren();
  panel.hidden = true;
  if (record?.prompt !== 'html' || !record.answer) return;
  panel.hidden = false;
  if (!/<(?:!doctype\s+html|html|head|body|div|main|section|style|svg|canvas)\b/i.test(record.answer)) {
    status.textContent = '回答中未找到 HTML，请查看原始回答';
    stop.disabled = reload.disabled = true;
    return;
  }
  currentRecord = record;
  reload.disabled = false;
  startPreview();
}
