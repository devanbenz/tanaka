// Animated random-cycling kaomoji. Each kaomoji is a list of frames; the
// current kaomoji's frames advance quickly, and a different random kaomoji is
// chosen every few seconds.
const KAOMOJI = [
  ["┌(・o・)┘", "└(・o・)┐", "┌(・o・)┐", "└(・o・)┘"],
  ["(・_・)", "(・_・ )", "( ・_・)", "(・_・)"],
  ["┐(・ω・)┌", "┌(・ω・)┐"],
  ["(>_<)", "(>ω<)", "(>＿<)"],
];

function startKaomoji(el) {
  let cur = Math.floor(Math.random() * KAOMOJI.length);
  let frame = 0;
  let last = Date.now();
  function tick() {
    if (Date.now() - last > 3000) {
      let k;
      do { k = Math.floor(Math.random() * KAOMOJI.length); } while (KAOMOJI.length > 1 && k === cur);
      cur = k; frame = 0; last = Date.now();
    }
    const frames = KAOMOJI[cur];
    el.textContent = frames[frame % frames.length];
    frame++;
  }
  tick();
  return setInterval(tick, 180);
}

document.addEventListener("DOMContentLoaded", function () {
  document.querySelectorAll(".kaomoji").forEach(startKaomoji);
});
window.startKaomoji = startKaomoji;
