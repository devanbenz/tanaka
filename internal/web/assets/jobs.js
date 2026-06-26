// Global job poller: shows a toast when a job completes, and drives the
// in-progress pages' redirect via window.__pollJob.
function showToast(msg) {
  let host = document.getElementById("toast-host");
  if (!host) {
    host = document.createElement("div");
    host.id = "toast-host";
    host.style.cssText = "position:fixed;bottom:12px;right:12px;z-index:1000;";
    document.body.appendChild(host);
  }
  const t = document.createElement("div");
  t.className = "window";
  t.style.cssText = "margin-top:8px;max-width:260px;";
  t.innerHTML = '<div class="title-bar"><div class="title-bar-text">Tanaka</div></div>' +
    '<div class="window-body" style="margin:6px"></div>';
  t.querySelector(".window-body").textContent = msg;
  host.appendChild(t);
  setTimeout(function () { t.remove(); }, 8000);
}

function announced() {
  try { return new Set(JSON.parse(localStorage.getItem("tanakaAnnounced") || "[]")); }
  catch (e) { return new Set(); }
}
function remember(set) {
  localStorage.setItem("tanakaAnnounced", JSON.stringify(Array.from(set)));
}

async function pollJobs() {
  let jobs;
  try {
    const res = await fetch("/jobs");
    if (!res.ok) return;
    jobs = await res.json();
  } catch (e) { return; }

  const seen = announced();
  for (const j of jobs) {
    if (!j.done) continue;
    const stamp = j.key + ":" + j.status;
    if (!seen.has(stamp)) {
      seen.add(stamp);
      const what = j.kind === "build" ? "build" : "study package";
      showToast(j.status === "error" ? (j.kind + " failed: " + j.error) : (what + " ready"));
    }
  }
  remember(seen);

  // In-progress page redirect.
  if (window.__pollJob) {
    const me = jobs.find(function (x) { return x.key === window.__pollJob.key; });
    if (me) {
      const p = document.getElementById("progress");
      if (p && me.progress) p.textContent = me.progress;
      if (me.done) {
        if (me.status === "done") { location.href = window.__pollJob.redirect; }
        else { location.reload(); }
      }
    }
  }
}

setInterval(pollJobs, 2000);
document.addEventListener("DOMContentLoaded", pollJobs);
