// dashboard/static/theme.js
(function () {
  var root = document.documentElement;
  var stored = localStorage.getItem('comqtt-theme');
  if (stored === 'dark') root.classList.add('dark');
  document.addEventListener('click', function (e) {
    if (!e.target || e.target.id !== 'theme-toggle') return;
    root.classList.toggle('dark');
    localStorage.setItem('comqtt-theme', root.classList.contains('dark') ? 'dark' : 'light');
  });
})();
