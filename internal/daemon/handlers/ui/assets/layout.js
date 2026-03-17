// Lucide Icons Integration (Local)
(function() {
  const script = document.createElement('script');
  script.src = '/ui/assets/lucide.min.js';
  script.onload = () => window.updateIcons && window.updateIcons();
  document.head.appendChild(script);
})();

window.updateIcons = function() {
  if (window.lucide) {
    window.lucide.createIcons();
  }
};

// ── Auth ──────────────────────────────────────────────────────────────────────

// Migrate stale credential keys from older builds
sessionStorage.removeItem('kompakt_creds');
sessionStorage.removeItem('sear_creds');

// authHeader returns the Authorization header value (Bearer JWT), or null.
function authHeader() {
  var t = sessionStorage.getItem('kompakt_token');
  return t ? 'Bearer ' + t : null;
}

function headersJSON() {
  var h = {'Content-Type': 'application/json'};
  var a = authHeader();
  if (a) h['Authorization'] = a;
  return h;
}

function headersAuth() {
  var h = {};
  var a = authHeader();
  if (a) h['Authorization'] = a;
  return h;
}

// ── Login overlay ─────────────────────────────────────────────────────────────

function ensureLoginOverlay() {
  if (document.getElementById('login-overlay')) return;
  const overlay = document.createElement('div');
  overlay.id = 'login-overlay';
  overlay.innerHTML = `
    <div id="login-box">
      <h2>&#x1F512; Kompakt Root Login</h2>
      <label>Username</label>
      <input id="lu" type="text" value="root" readonly>
      <label>Password</label>
      <input id="lp" type="password" autocomplete="current-password" placeholder="root password">
      <div id="login-error"></div>
      <button class="btn-login" onclick="window.doLogin()">Sign in</button>
    </div>
  `;
  document.body.appendChild(overlay);
}

function showLogin(msg) {
  ensureLoginOverlay();
  const errEl = document.getElementById('login-error');
  if (errEl) errEl.textContent = msg || '';
  const lp = document.getElementById('lp');
  if (lp) lp.value = '';
  const overlay = document.getElementById('login-overlay');
  if (overlay) overlay.classList.add('show');
  setTimeout(function() {
    const lpf = document.getElementById('lp');
    if (lpf) lpf.focus();
  }, 50);
}

function hideLogin() {
  const overlay = document.getElementById('login-overlay');
  if (overlay) overlay.classList.remove('show');
}

function logout() {
  sessionStorage.removeItem('kompakt_token');
  showLogin('');
}

function initLogin(apiPath, onSuccess) {
  const cb = onSuccess || (() => typeof load === 'function' ? load() : null);

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

  const pwField = document.getElementById('lp');
  if (pwField) pwField.addEventListener('keydown', function(e) { if (e.key === 'Enter') doLogin(); });

  window.doLogin = doLogin;

  if (authHeader()) {
    hideLogin();
    cb();
  } else {
    showLogin('');
  }
}

// ── Confirm modal ─────────────────────────────────────────────────────────────

var _confirmCb = null;

function ensureConfirmModal() {
  if (document.getElementById('modal-confirm-overlay')) return;
  const el = document.createElement('div');
  el.id = 'modal-confirm-overlay';
  el.className = 'modal-bg';
  el.innerHTML = `
    <div class="modal" style="max-width:380px">
      <h3 id="modal-confirm-title" style="margin-bottom:12px">Confirm</h3>
      <p id="modal-confirm-msg" style="color:#8b949e;font-size:.875rem;margin-bottom:20px"></p>
      <div class="modal-actions">
        <button onclick="closeConfirm()">Cancel</button>
        <button class="btn-danger" id="modal-confirm-btn" onclick="doConfirm()">Delete</button>
      </div>
    </div>
  `;
  document.body.appendChild(el);
}

function showConfirm(title, message, cb, btnLabel) {
  ensureConfirmModal();
  _confirmCb = cb;
  document.getElementById('modal-confirm-title').textContent = title;
  document.getElementById('modal-confirm-msg').textContent = message;
  var btn = document.getElementById('modal-confirm-btn');
  if (btn) btn.textContent = btnLabel || 'Delete';
  document.getElementById('modal-confirm-overlay').classList.add('show');
}

function closeConfirm() {
  _confirmCb = null;
  var el = document.getElementById('modal-confirm-overlay');
  if (el) el.classList.remove('show');
}

function doConfirm() {
  var cb = _confirmCb;
  closeConfirm();
  if (cb) cb();
}

// ── Pagination ────────────────────────────────────────────────────────────────

var PAGE_SIZE = 25;

function buildPagination(total, page) {
  var pages = Math.ceil(total / PAGE_SIZE);
  if (pages <= 1) return '';
  return '<div class="pagination">' +
    '<button onclick="setPage(' + (page - 1) + ')" ' + (page <= 1 ? 'disabled' : '') + '>&#8592; Prev</button>' +
    '<span>Page ' + page + ' / ' + pages + '</span>' +
    '<button onclick="setPage(' + (page + 1) + ')" ' + (page >= pages ? 'disabled' : '') + '>Next &#8594;</button>' +
    '</div>';
}

// ── XSS helper ────────────────────────────────────────────────────────────────

function esc(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// ── Global event delegation ───────────────────────────────────────────────────

document.addEventListener('click', function(e) {
  // Close confirm modal on backdrop click
  if (e.target && e.target.id === 'modal-confirm-overlay') { closeConfirm(); return; }

  // Menu toggle (delegated — covers dynamically rendered rows)
  const menuBtn = e.target.closest('.menu-btn');
  if (menuBtn) {
    e.stopPropagation();
    const dropdown = menuBtn.nextElementSibling;
    const wasOpen = dropdown && dropdown.classList.contains('open');
    document.querySelectorAll('.menu-dropdown.open').forEach(d => d.classList.remove('open'));
    if (dropdown && !wasOpen) dropdown.classList.add('open');
    return;
  }
  document.querySelectorAll('.menu-dropdown.open').forEach(d => d.classList.remove('open'));
});

document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') { closeConfirm(); }
  if (e.key === '/' && document.activeElement.tagName !== 'INPUT' && document.activeElement.tagName !== 'TEXTAREA') {
    const q = document.getElementById('f-query');
    if (q) { e.preventDefault(); q.focus(); }
  }
});
