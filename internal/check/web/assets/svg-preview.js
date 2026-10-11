let activeURL;
export function extractSVG(answer) {
  const start = answer.search(/<svg\b/i);
  if (start < 0) return undefined;
  const end = answer.toLowerCase().indexOf('</svg>', start);
  if (end < 0) throw Error('SVG 尚未完整返回，请查看原始回答。');
  const document = new DOMParser().parseFromString(answer.slice(start, end + 6), 'image/svg+xml');
  if (document.querySelector('parsererror') || document.documentElement.localName !== 'svg') throw Error('SVG 格式无效，请查看原始回答。');
  const root = document.documentElement;
  // SVG image mode already disables scripts; remove active and remote content
  // as well so generated answers never become page markup or network clients.
  root.querySelectorAll('script,foreignObject,iframe,object,embed,image,a,animate,set,animateMotion,animateTransform').forEach(node => node.remove());
  root.querySelectorAll('style').forEach(node => {
    if (/@import|url\s*\(\s*['"]?(?!#)/i.test(node.textContent)) node.remove();
  });
  for (const element of [root, ...root.querySelectorAll('*')]) {
    for (const attribute of [...element.attributes]) {
      const name = attribute.localName.toLowerCase();
      if (name.startsWith('on') || name === 'src' || (name === 'href' && !attribute.value.startsWith('#')) || /url\s*\(\s*['"]?(?!#)|@import/i.test(attribute.value)) element.removeAttributeNode(attribute);
    }
  }
  root.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  return new XMLSerializer().serializeToString(root);
}
export function renderSVGPreview(record) {
  const panel = document.querySelector('#svgPreview');
  const image = document.querySelector('#svgImage');
  const status = document.querySelector('#svgPreviewStatus');
  image.onload = image.onerror = null;
  image.removeAttribute('src');
  if (activeURL) URL.revokeObjectURL(activeURL);
  activeURL = undefined;
  panel.hidden = true;
  if (!record?.answer || !['svg', 'html'].includes(record.prompt)) return;
  try {
    const svg = extractSVG(record.answer);
    if (!svg) return;
    panel.hidden = false;
    status.textContent = '正在渲染…';
    image.onload = () => { status.textContent = '静态预览；脚本、外部资源和动画不执行。'; };
    image.onerror = () => { status.textContent = '无法渲染 SVG，请查看原始回答。'; };
    activeURL = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
    image.src = activeURL;
  } catch (error) { panel.hidden = false; status.textContent = error.message; }
}
