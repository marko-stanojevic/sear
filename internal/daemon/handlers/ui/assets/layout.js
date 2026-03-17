// Lucide Icons Integration (Local)
(function() {
  const script = document.createElement('script');
  script.src = '/ui/assets/lucide.min.js';
  script.onload = () => window.updateIcons && window.updateIcons();
  document.head.appendChild(script);
})();

window.updateIcons = function() {
  if (window.lucide) window.lucide.createIcons();
};

// ── Header ────────────────────────────────────────────────────────────────────

const _SVG_LOGO = '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#f78166" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:text-bottom;margin-right:4px"><path d="M13 2 L3 14 h9 l-1 8 10-12 h-9 l1-8z"></path></svg>';
const _NAV_PAGES = [
  ['Home',        '/ui'],
  ['Clients',     '/ui/clients'],
  ['Secrets',     '/ui/secrets'],
  ['Playbooks',   '/ui/playbooks'],
  ['Deployments', '/ui/deployments'],
  ['Artifacts',   '/ui/artifacts'],
];

/**
 * buildHeader renders the shared app header into the <header> element.
 * @param {string} activePage       - Nav label to mark active, e.g. 'Clients'
 * @param {string} [searchPlaceholder] - If provided, adds the search input
 */
function buildHeader(activePage, searchPlaceholder) {
  var navHTML = _NAV_PAGES.map(function(item) {
    return '<a href="' + item[1] + '"' + (activePage === item[0] ? ' class="active"' : '') + '>' + item[0] + '</a>';
  }).join('');

  var searchHTML = searchPlaceholder
    ? '<div class="header-search"><input id="f-query" type="text" placeholder="' + searchPlaceholder + '" autocomplete="off"><button id="f-clear" class="btn-clear btn-muted" onclick="clearSearch()" title="Clear search">\u2715</button></div>'
    : '';

  var header = document.querySelector('header');
  if (!header) return;
  header.innerHTML =
    '<div class="header-brand"><h1>' + _SVG_LOGO + ' Kompakt</h1><span class="badge">DASHBOARD</span></div>' +
    '<nav>' + navHTML + '</nav>' +
    searchHTML +
    '<div class="header-right"><button onclick="logout()">Sign out</button></div>';

  // Wire the search input — render/updateClearBtn are defined in page scripts (hoisted)
  if (searchPlaceholder) {
    var q = document.getElementById('f-query');
    if (q) q.addEventListener('input', function() {
      currentPage = 1;
      if (typeof render === 'function') render();
      updateClearBtn();
    });
  }
}

// ── Search helpers ────────────────────────────────────────────────────────────

function clearSearch() {
  var q = document.getElementById('f-query');
  if (q) q.value = '';
  currentPage = 1;
  if (typeof render === 'function') render();
  updateClearBtn();
}

function updateClearBtn() {
  var q   = document.getElementById('f-query');
  var btn = document.getElementById('f-clear');
  if (q && btn) btn.classList.toggle('show', !!q.value);
}

// ── Pagination ────────────────────────────────────────────────────────────────

var PAGE_SIZE   = 25;
var currentPage = 1;

function setPage(n) {
  currentPage = n;
  if (typeof render === 'function') render();
}

function buildPagination(total, page) {
  var pages = Math.ceil(total / PAGE_SIZE);
  if (pages <= 1) return '';
  return '<div class="pagination">' +
    '<button onclick="setPage(' + (page - 1) + ')" ' + (page <= 1 ? 'disabled' : '') + '>&#8592; Prev</button>' +
    '<span>Page ' + page + ' / ' + pages + '</span>' +
    '<button onclick="setPage(' + (page + 1) + ')" ' + (page >= pages ? 'disabled' : '') + '>Next &#8594;</button>' +
    '</div>';
}

// ── Auto-refresh ──────────────────────────────────────────────────────────────

var _autoRefreshEnabled = false;
var _autoRefreshTimer   = null;
var _autoRefreshMs      = 10000;

function startAutoRefresh(ms) {
  _autoRefreshEnabled = true;
  _autoRefreshMs = ms || 10000;
  stopAutoRefresh();
  _autoRefreshTimer = setInterval(function() { if (typeof load === 'function') load(); }, _autoRefreshMs);
}

function stopAutoRefresh() {
  if (_autoRefreshTimer) { clearInterval(_autoRefreshTimer); _autoRefreshTimer = null; }
}

// ── Auth ──────────────────────────────────────────────────────────────────────

// Purge stale credential keys from older builds
sessionStorage.removeItem('kompakt_creds');
sessionStorage.removeItem('sear_creds');

function authHeader() {
  var t = sessionStorage.getItem('kompakt_token');
  return t ? 'Bearer ' + t : null;
}

function headersJSON() {
  var h = {'Content-Type': 'application/json'};
  var a = authHeader(); if (a) h['Authorization'] = a;
  return h;
}

function headersAuth() {
  var h = {};
  var a = authHeader(); if (a) h['Authorization'] = a;
  return h;
}

// ── Login overlay ─────────────────────────────────────────────────────────────

function ensureLoginOverlay() {
  if (document.getElementById('login-overlay')) return;
  var overlay = document.createElement('div');
  overlay.id = 'login-overlay';
  overlay.innerHTML =
    '<div id="login-box">' +
      '<h2>&#x1F512; Kompakt Root Login</h2>' +
      '<label>Username</label>' +
      '<input id="lu" type="text" value="root" readonly>' +
      '<label>Password</label>' +
      '<input id="lp" type="password" autocomplete="current-password" placeholder="root password">' +
      '<div id="login-error"></div>' +
      '<button class="btn-login" onclick="window.doLogin()">Sign in</button>' +
    '</div>';
  document.body.appendChild(overlay);
}

function showLogin(msg) {
  ensureLoginOverlay();
  var errEl = document.getElementById('login-error'); if (errEl) errEl.textContent = msg || '';
  var lp    = document.getElementById('lp');           if (lp) lp.value = '';
  var overlay = document.getElementById('login-overlay'); if (overlay) overlay.classList.add('show');
  setTimeout(function() { var lpf = document.getElementById('lp'); if (lpf) lpf.focus(); }, 50);
}

function hideLogin() {
  var overlay = document.getElementById('login-overlay');
  if (overlay) overlay.classList.remove('show');
}

function logout() { sessionStorage.removeItem('kompakt_token'); showLogin(''); }

function initLogin(apiPath, onSuccess) {
  var cb = onSuccess || function() { if (typeof load === 'function') load(); };

  async function doLogin() {
    var p = document.getElementById('lp').value;
    if (!p) { document.getElementById('login-error').textContent = 'Password required'; return; }
    document.getElementById('login-error').textContent = 'Signing in\u2026';
    try {
      var r = await fetch('/api/v1/ui/login', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({password: p})
      });
      if (r.status === 401) { document.getElementById('login-error').textContent = 'Invalid password'; return; }
      if (!r.ok) { document.getElementById('login-error').textContent = 'Error: ' + r.status; return; }
      var data = await r.json();
      sessionStorage.setItem('kompakt_token', data.token);
      hideLogin();
      cb();
    } catch (e) {
      document.getElementById('login-error').textContent = 'Network error';
    }
  }

  var pwField = document.getElementById('lp');
  if (pwField) pwField.addEventListener('keydown', function(e) { if (e.key === 'Enter') doLogin(); });
  window.doLogin = doLogin;

  if (authHeader()) { hideLogin(); cb(); } else { showLogin(''); }
}

// ── Confirm modal ─────────────────────────────────────────────────────────────

var _confirmCb = null;

function ensureConfirmModal() {
  if (document.getElementById('modal-confirm-overlay')) return;
  var el = document.createElement('div');
  el.id = 'modal-confirm-overlay';
  el.className = 'modal-bg';
  el.innerHTML =
    '<div class="modal" style="max-width:380px">' +
      '<h3 id="modal-confirm-title" style="margin-bottom:12px">Confirm</h3>' +
      '<p id="modal-confirm-msg" style="color:#8b949e;font-size:.875rem;margin-bottom:20px"></p>' +
      '<div class="modal-actions">' +
        '<button onclick="closeConfirm()">Cancel</button>' +
        '<button class="btn-danger" id="modal-confirm-btn" onclick="doConfirm()">Delete</button>' +
      '</div>' +
    '</div>';
  document.body.appendChild(el);
}

function showConfirm(title, message, cb, btnLabel) {
  ensureConfirmModal();
  _confirmCb = cb;
  document.getElementById('modal-confirm-title').textContent = title;
  document.getElementById('modal-confirm-msg').textContent   = message;
  var btn = document.getElementById('modal-confirm-btn');
  if (btn) btn.textContent = btnLabel || 'Delete';
  document.getElementById('modal-confirm-overlay').classList.add('show');
}

function closeConfirm() {
  _confirmCb = null;
  var el = document.getElementById('modal-confirm-overlay');
  if (el) el.classList.remove('show');
}

function doConfirm() { var cb = _confirmCb; closeConfirm(); if (cb) cb(); }

// ── XSS helper ────────────────────────────────────────────────────────────────

function esc(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

// ── Global event delegation ───────────────────────────────────────────────────

document.addEventListener('click', function(e) {
  if (e.target && e.target.id === 'modal-confirm-overlay') { closeConfirm(); return; }
  var menuBtn = e.target.closest('.menu-btn');
  if (menuBtn) {
    e.stopPropagation();
    var dropdown = menuBtn.nextElementSibling;
    var wasOpen  = dropdown && dropdown.classList.contains('open');
    document.querySelectorAll('.menu-dropdown.open').forEach(function(d) { d.classList.remove('open'); });
    if (dropdown && !wasOpen) dropdown.classList.add('open');
    return;
  }
  document.querySelectorAll('.menu-dropdown.open').forEach(function(d) { d.classList.remove('open'); });
});

document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') { closeConfirm(); }
  if (e.key === '/' && document.activeElement.tagName !== 'INPUT' && document.activeElement.tagName !== 'TEXTAREA') {
    var q = document.getElementById('f-query');
    if (q) { e.preventDefault(); q.focus(); }
  }
});

document.addEventListener('visibilitychange', function() {
  if (!_autoRefreshEnabled) return;
  if (document.hidden) { stopAutoRefresh(); }
  else { if (typeof load === 'function') load(); startAutoRefresh(_autoRefreshMs); }
});
