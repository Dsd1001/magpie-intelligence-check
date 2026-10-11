import { icons } from './icons.js';

export const $ = selector => document.querySelector(selector);
export function text(tag, value, className) {
  const node = document.createElement(tag);
  node.textContent = value;
  if (className) node.className = className;
  return node;
}
export function icon(name) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  for (const [key, value] of Object.entries({ width: '18', height: '18', viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': '1.5', 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true' })) svg.setAttribute(key, value);
  for (const [tag, attributes] of icons[name] || []) {
    const node = document.createElementNS(svg.namespaceURI, tag);
    for (const [key, value] of Object.entries(attributes)) node.setAttribute(key, value);
    svg.append(node);
  }
  return svg;
}
export function hydrateIcons(root = document) {
  root.querySelectorAll('[data-icon]').forEach(node => node.replaceChildren(icon(node.dataset.icon)));
}
export function syncSelect(select, options) {
  const signature = JSON.stringify(options);
  if (select.dataset.options === signature) return;
  const selected = select.value;
  select.replaceChildren(...options.map(([value, label]) => {
    const option = text('option', label);
    option.value = value;
    return option;
  }));
  if (options.some(([value]) => value === selected)) select.value = selected;
  select.dataset.options = signature;
}
export const dateTime = raw => Date.parse(raw) > 0 ? new Date(raw).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) : '未通过';
export const names = { unverified: '未检测', checking: '检测中', passed: '通过', review: '人工确认', unavailable: '检测不可用', manual: '待人工评价', failed: '未通过', running: '进行中' };
