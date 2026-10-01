// Renders the latest release (version, build, size, checksum, components) and the
// release list from window.AMPLS_RELEASES (releases.js, written by
// scripts/publish-site.ps1). The HTML already holds the current values, so the
// page still works when this script cannot run.
(function () {
  var data = window.AMPLS_RELEASES;
  if (!data || !data.releases || !data.releases.length) return;
  var latest = data.releases[0];

  var NAMES = { apache: 'Apache', mod_fcgid: 'mod_fcgid', php: 'PHP', mysql: 'MySQL', phpmyadmin: 'phpMyAdmin', composer: 'Composer' };

  function mb(bytes) { return Math.round(bytes / 1048576) + ' MB'; }
  function date(iso, short) {
    var d = new Date(iso + 'T00:00:00');
    if (isNaN(d)) return iso;
    return d.toLocaleDateString('en-GB', { day: 'numeric', month: short ? 'short' : 'long', year: 'numeric' });
  }
  function text(tag, value) { var el = document.createElement(tag); el.textContent = value; return el; }

  var values = {
    version: latest.version,
    build: String(latest.build),
    sizeText: mb(latest.size),
    dateText: date(latest.date),
    fileName: latest.file.split('/').pop(),
    sha256: latest.sha256,
  };
  document.querySelectorAll('[data-r]').forEach(function (el) {
    var v = values[el.getAttribute('data-r')];
    if (v != null) el.textContent = v;
  });
  document.querySelectorAll('[data-r-href="file"]').forEach(function (a) { a.setAttribute('href', latest.file); });

  // Bundled components
  var list = document.querySelector('[data-r-list="components"]');
  if (list && latest.components) {
    list.textContent = '';
    ['apache', 'php', 'mysql', 'phpmyadmin', 'composer'].forEach(function (k) {
      if (!latest.components[k]) return;
      var li = document.createElement('li');
      li.appendChild(text('span', NAMES[k] || k));
      li.appendChild(text('b', latest.components[k]));
      list.appendChild(li);
    });
  }

  // All releases
  var body = document.querySelector('[data-r-table="releases"]');
  if (body) {
    body.textContent = '';
    data.releases.forEach(function (r, i) {
      var tr = document.createElement('tr');
      var v = text('td', r.version);
      if (i === 0) v.appendChild(text('span', 'Latest')).className = 'tag';
      tr.appendChild(v);
      tr.appendChild(text('td', String(r.build)));
      tr.appendChild(text('td', date(r.date, true)));
      tr.appendChild(text('td', mb(r.size)));
      var td = document.createElement('td');
      var a = text('a', 'Download');
      a.href = r.file;
      a.setAttribute('download', '');
      a.setAttribute('aria-label', 'Download AMPLS ' + r.version);
      td.appendChild(a);
      tr.appendChild(td);
      body.appendChild(tr);
    });
  }

  // Copy checksum
  document.querySelectorAll('[data-copy]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var v = values[btn.getAttribute('data-copy')];
      if (!navigator.clipboard) return;
      navigator.clipboard.writeText(v).then(function () {
        btn.textContent = 'Copied';
        setTimeout(function () { btn.textContent = 'Copy'; }, 1500);
      });
    });
  });
})();
