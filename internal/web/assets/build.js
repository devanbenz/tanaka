async function runTests(id, lang) {
  const out = document.getElementById('output');
  out.textContent = 'running tests...';
  try {
    const res = await fetch('/build/' + id + '/' + lang + '/test', { method: 'POST' });
    const v = await res.json();
    out.textContent = (v.runError ? '[could not run] ' : (v.passed ? '[passed] ' : '[failed] ')) + (v.output || '');
    if (v.passed) { location.reload(); }
  } catch (e) { out.textContent = 'error running tests'; }
}
async function getHint(id, lang) {
  const hint = document.getElementById('hint');
  const out = document.getElementById('output');
  hint.textContent = 'thinking...';
  try {
    const res = await fetch('/build/' + id + '/' + lang + '/hint', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ output: out.textContent }),
    });
    if (!res.ok) { hint.textContent = 'hint unavailable'; return; }
    const v = await res.json();
    hint.textContent = 'hint: ' + v.hint;
  } catch (e) { hint.textContent = 'hint unavailable'; }
}
