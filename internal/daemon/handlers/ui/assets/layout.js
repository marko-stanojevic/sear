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

function ensureLoginOverlay() {
  if (document.getElementById('login-overlay')) return;
  const overlay = document.createElement('div');
  overlay.id = 'login-overlay';
  overlay.innerHTML = `
    <div id="login-box">
      <h2>&#x1F512; Sear Root Login</h2>
      <label>Username</label>
      <input id="lu" type="text" value="root" readonly>
      <label>Password</label>
      <input id="lp" type="password" autocomplete="current-password" placeholder="root password">
      <div id="login-error"></div>
      <button class="btn-login" onclick="window.doLogin()">Sign in</button>
    </div>
  `;
  overlay.style.display = 'none'; // Ensure it's hidden during construction
  document.body.appendChild(overlay);
}

function authHeader(){var c=sessionStorage.getItem('sear_creds');return c?'Basic '+c:null;}

function showLogin(msg) {
  ensureLoginOverlay();
  const errEl = document.getElementById('login-error');
  if (errEl) errEl.textContent = msg || '';
  const lp = document.getElementById('lp');
  if (lp) lp.value = '';
  const overlay = document.getElementById('login-overlay');
  if (overlay) overlay.classList.add('show');
  setTimeout(function(){
    const lpf = document.getElementById('lp');
    if (lpf) lpf.focus();
  }, 50);
}
function hideLogin(){
  const overlay = document.getElementById('login-overlay');
  if (overlay) overlay.classList.remove('show');
}
function logout(){sessionStorage.removeItem('sear_creds');showLogin('');}

function initLogin(apiPath, onSuccess) {
  const cb = onSuccess || (() => typeof load === 'function' ? load() : null);

  async function doLogin(){
    var u=document.getElementById('lu').value;
    var p=document.getElementById('lp').value;
    if(!p){document.getElementById('login-error').textContent='Password required';return;}
    var creds=btoa(u+':'+p);
    document.getElementById('login-error').textContent='Signing in...';
    try {
      var r=await fetch(apiPath,{headers:{Authorization:'Basic '+creds}});
      if(r.status===401){document.getElementById('login-error').textContent='Invalid password';return;}
      if(!r.ok){document.getElementById('login-error').textContent='Error: '+r.status;return;}
      sessionStorage.setItem('sear_creds',creds);
      hideLogin();
      cb();
    } catch(e) {
      document.getElementById('login-error').textContent='Network error';
    }
  }

  const pwField = document.getElementById('lp');
  if (pwField) pwField.addEventListener('keydown',function(e){if(e.key==='Enter')doLogin();});
  
  window.doLogin = doLogin;
  
  if (authHeader()) {
    // If credentials exist, hide the login box and load the page
    hideLogin();
    cb();
  } else {
    showLogin('');
  }
}

function headersJSON(){var h={'Content-Type':'application/json'};var a=authHeader();if(a)h['Authorization']=a;return h;}
function headersAuth(){var h={};var a=authHeader();if(a)h['Authorization']=a;return h;}

function esc(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

document.addEventListener('click', function(e) {
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
  if (e.key === '/' && document.activeElement.tagName !== 'INPUT' && document.activeElement.tagName !== 'TEXTAREA') {
    const q = document.getElementById('f-query');
    if (q) {
      e.preventDefault();
      q.focus();
    }
  }
});
