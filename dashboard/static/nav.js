// nav.js - mobile-friendly nav drawer.
// On screens < lg the sidebar is off-canvas. The top-bar hamburger toggles
// it; clicking the backdrop, a nav link, or the in-nav close button hides
// it again.
(function () {
  function nav()      { return document.getElementById('primary-nav'); }
  function backdrop() { return document.getElementById('nav-backdrop'); }
  function open()  { nav().classList.remove('-translate-x-full'); backdrop().classList.remove('hidden'); }
  function close() { nav().classList.add('-translate-x-full');    backdrop().classList.add('hidden'); }

  document.addEventListener('click', function (e) {
    if (!nav() || !backdrop()) return;
    var t = e.target;
    if (t.closest && t.closest('#nav-toggle')) { open();  e.preventDefault(); return; }
    if (t.closest && t.closest('#nav-close'))  { close(); e.preventDefault(); return; }
    if (t.id === 'nav-backdrop')               { close(); return; }
    // Auto-collapse on link clicks (only matters on mobile).
    if (t.closest && t.closest('.nav-link') && window.innerWidth < 1024) close();
  });

  // Esc closes the drawer.
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') close();
  });
})();
