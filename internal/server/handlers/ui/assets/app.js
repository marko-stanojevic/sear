// ── HTMX auth hook ────────────────────────────────────────────────────────────
// Inject the JWT on every HTMX request so partials (protected by RequireRootAuth) work.
document.addEventListener('htmx:configRequest', function(e) {
  var token = localStorage.getItem('kompakt_token');
  if (token) e.detail.headers['Authorization'] = 'Bearer ' + token;
});

// Intercept 401 responses from HTMX: cancel the swap and show the login modal.
document.addEventListener('htmx:beforeSwap', function(e) {
  if (e.detail.xhr.status === 401) {
    e.detail.shouldSwap = false;
    var hadToken = !!localStorage.getItem('kompakt_token');
    if (hadToken) {
      localStorage.removeItem('kompakt_token');
      showLogin('Session expired — sign in again');
    }
    // else: no token, login modal is already showing
  }
});

// Re-run Lucide after every HTMX swap.
document.addEventListener('htmx:afterSwap', function() {
  if (window.lucide) window.lucide.createIcons();
});

// ── Auth helpers (used by JS mutation functions) ───────────────────────────────
function headersAuth() {
  var h = {'Accept': 'application/json'};
  var t = localStorage.getItem('kompakt_token');
  if (t) h['Authorization'] = 'Bearer ' + t;
  return h;
}
function headersJSON() {
  var h = headersAuth();
  h['Content-Type'] = 'application/json';
  return h;
}

// ── Login modal ───────────────────────────────────────────────────────────────
function showLogin(msg) {
  var modal = document.getElementById('login-modal');
  var msgEl = document.getElementById('login-msg');
  if (!modal) return;
  if (msg) { msgEl.textContent = msg; msgEl.style.display = 'block'; }
  else      { msgEl.style.display = 'none'; }
  modal.classList.add('show');
  setTimeout(function() {
    var pw = document.getElementById('login-pw');
    if (pw) { pw.value = ''; pw.focus(); }
  }, 50);
}

function hideLogin() {
  var modal = document.getElementById('login-modal');
  if (modal) modal.classList.remove('show');
}

async function doLogin() {
  var pw = document.getElementById('login-pw');
  var msgEl = document.getElementById('login-msg');
  if (!pw || !pw.value) { msgEl.textContent = 'Password required'; msgEl.style.display = 'block'; return; }
  try {
    var r = await fetch('/api/v1/ui/login', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({password: pw.value})
    });
    if (!r.ok) {
      msgEl.textContent = 'Invalid password';
      msgEl.style.display = 'block';
      pw.value = '';
      pw.focus();
      return;
    }
    var data = await r.json();
    localStorage.setItem('kompakt_token', data.token);
    location.reload();
  } catch(e) {
    msgEl.textContent = 'Network error';
    msgEl.style.display = 'block';
  }
}

// Allow Enter key in login modal.
document.addEventListener('keydown', function(e) {
  if (e.key === 'Enter' && document.getElementById('login-modal').classList.contains('show')) {
    doLogin();
  }
});

// Check token validity on page load — if no token show login immediately.
(function() {
  var token = localStorage.getItem('kompakt_token');
  if (!token) {
    showLogin('');
  }
})();

function logout() {
  localStorage.removeItem('kompakt_token');
  showLogin('');
}

// ── Confirm modal ─────────────────────────────────────────────────────────────
var _confirmCb = null;
function showConfirm(title, msg, cb) {
  _confirmCb = cb;
  document.getElementById('confirm-title').textContent = title;
  document.getElementById('confirm-msg').textContent = msg;
  document.getElementById('confirm-modal').classList.add('show');
}
function dismissConfirm() {
  document.getElementById('confirm-modal').classList.remove('show');
  _confirmCb = null;
}
document.getElementById('confirm-ok').addEventListener('click', function() {
  document.getElementById('confirm-modal').classList.remove('show');
  if (_confirmCb) { _confirmCb(); _confirmCb = null; }
});

// ── Utilities ─────────────────────────────────────────────────────────────────
function esc(s) {
  return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// Run Lucide on initial load.
document.addEventListener('DOMContentLoaded', function() {
  if (window.lucide) window.lucide.createIcons();
});
