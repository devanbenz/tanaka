// Live quiz grading: POST an answer to /grade and show the verdict.
async function grade(form) {
  const data = {
    questionId: form.dataset.qid,
    choice: form.querySelector('input[type=radio]:checked')
      ? parseInt(form.querySelector('input[type=radio]:checked').value, 10)
      : -1,
    answer: form.querySelector('textarea') ? form.querySelector('textarea').value : "",
  };
  const out = form.querySelector('.verdict');
  out.textContent = 'grading...';
  try {
    const res = await fetch('/grade', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    if (!res.ok) { out.textContent = 'grading unavailable - try again'; return; }
    const v = await res.json();
    out.textContent = v.verdict + (v.feedback ? ' - ' + v.feedback : '');
    if (v.sectionPassed) {
      const next = document.getElementById('next-btn');
      if (next) next.disabled = false;
    }
  } catch (e) {
    out.textContent = 'grading unavailable - try again';
  }
}
document.addEventListener('submit', function (e) {
  if (e.target.classList.contains('quiz-form')) { e.preventDefault(); grade(e.target); }
});
