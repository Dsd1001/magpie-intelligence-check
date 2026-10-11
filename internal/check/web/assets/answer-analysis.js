// Observable checks only: no model attribution or generated-code execution.
export function analyzeAnswer(record, prompt) {
  const answer = record.answer || '';
  const checks = [];
  if (!answer) return checks;
  checks.push(['回答长度', `${Array.from(answer).length} 字符 / ${answer.split(/\r?\n/).length} 行`]);
  if (record.error) checks.push(['完整性', '调用异常或截断，请结合已返回正文审核']);
  if (record.prompt === 'repeat') {
    const items = [...(prompt?.text || record.prompt_text || '').matchAll(/^\d+\. (.+)$/gm)].map(match => match[1].trim());
    const missing = items.filter(item => !answer.includes(item));
    if (items.length) {
      checks.push(['逐字覆盖', `${items.length - missing.length}/${items.length} 个条目包含完整原文`]);
      if (missing.length) checks.push(['未完整出现', missing.join('\n')]);
      checks.push(['评价边界', '包含原文不代表涵义解释正确，也不能识别模型身份']);
    }
  } else if (record.prompt === 'milk') {
    checks.push(['自动题结论', record.verdict === 'passed' ? '最终数字符合 46.24 判定规则' : '未通过最终数字判定；请核对推理与答案']);
  } else if (record.prompt === 'html' || record.prompt === 'svg') {
    checks.push(['输出结构线索', /<svg\b/i.test(answer) ? '检测到 SVG 标记' : '未检测到 SVG 标记']);
    checks.push(['评价边界', '仅检查文本标记，未执行代码；画面、结构与动画仍需人工确认']);
  } else {
    checks.push(['评价边界', '本题没有可靠的自动评分标准，请根据参考说明审核正文']);
  }
  return checks;
}
