package handlers

import (
	"fmt"
	"net/http"
)

// HandleDeploymentsUI serves the deployments page.
func (e *Env) HandleDeploymentsUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	fmt.Fprint(w, deploymentsHTML)
}

const deploymentsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sear - Deployments</title>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:'Segoe UI',system-ui,sans-serif;background:#0d1117;color:#e6edf3;min-height:100vh}
  header{background:#161b22;border-bottom:1px solid #30363d;padding:0 24px;display:flex;align-items:stretch;gap:0}
  .header-brand{display:flex;align-items:center;gap:12px;padding:16px 0}
  header h1{font-size:1.25rem;font-weight:600}
  nav{display:flex;gap:2px;align-items:stretch;margin-left:24px}
  nav a{color:#8b949e;text-decoration:none;padding:0 14px;display:flex;align-items:center;font-size:.875rem;border-bottom:2px solid transparent}
  nav a:hover{color:#e6edf3}
  nav a.active{color:#e6edf3;border-bottom-color:#f78166}
  .header-right{margin-left:auto;display:flex;align-items:center}
  .container{max-width:1300px;margin:0 auto;padding:24px}
  .meta{color:#8b949e;font-size:.875rem;margin-bottom:14px;display:flex;align-items:center;gap:12px}
  .meta .spacer{margin-left:auto}
  .filters{display:grid;grid-template-columns:minmax(240px,1fr) 180px auto;gap:10px;margin-bottom:14px}
  .filters input,.filters select{background:#0d1117;border:1px solid #30363d;color:#e6edf3;padding:8px 10px;border-radius:6px;font-size:.85rem;outline:none}
  .filters input:focus,.filters select:focus{border-color:#58a6ff}
  .list{background:#161b22;border:1px solid #30363d;border-radius:8px;overflow:hidden}
  .list-head,.list-row{display:grid;grid-template-columns:1fr 1fr 1fr .8fr .7fr .9fr auto;gap:10px;align-items:center;padding:10px 12px}
  .list-head{background:#0d1117;color:#8b949e;font-size:.75rem;text-transform:uppercase;letter-spacing:.04em;border-bottom:1px solid #30363d}
  .list-row{border-bottom:1px solid #21262d;font-size:.875rem}
  .list-row:last-child{border-bottom:none}
  .mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.8rem;color:#8b949e;word-break:break-all}
  .pill{display:inline-flex;align-items:center;gap:6px;font-size:.82rem;padding:3px 10px;border-radius:12px;font-weight:500}
  .pill.running{background:#1f6feb33;color:#58a6ff}
  .pill.done{background:#23863633;color:#3fb950}
  .pill.failed{background:#da363333;color:#f85149}
  .pill.rebooting{background:#9e6a0333;color:#e3b341}
  .pill.pending{background:#21262d;color:#8b949e}
  .dot{width:7px;height:7px;border-radius:50%;background:currentColor}
  button{background:#21262d;border:1px solid #30363d;color:#e6edf3;padding:6px 12px;border-radius:6px;cursor:pointer;font-size:.85rem}
  button:hover{background:#30363d}
  .empty{text-align:center;color:#8b949e;padding:60px 20px}
  .panel{margin-top:16px;background:#161b22;border:1px solid #30363d;border-radius:8px;overflow:hidden}
  .panel-head{background:#0d1117;border-bottom:1px solid #30363d;padding:10px 12px;color:#8b949e;font-size:.8rem;display:flex;justify-content:space-between;align-items:center}
  .log-head{display:grid;grid-template-columns:.9fr .6fr .7fr .9fr 2fr;gap:10px;padding:8px 12px;border-bottom:1px solid #30363d;background:#0d1117;color:#8b949e;font-size:.74rem;text-transform:uppercase;letter-spacing:.04em}
  .log-list{max-height:360px;overflow:auto}
  .log-row{display:grid;grid-template-columns:.9fr .6fr .7fr .9fr 2fr;gap:10px;padding:8px 12px;border-bottom:1px solid #21262d;font-size:.82rem}
  .log-row:last-child{border-bottom:none}
  .log-row .ts{color:#8b949e}
  .log-row .msg{white-space:pre-wrap;word-break:break-word}
  @media (max-width: 1100px){
    .list-head{display:none}
    .list-row{grid-template-columns:1fr;gap:6px}
    .log-head{display:none}
    .log-row{grid-template-columns:1fr;gap:4px}
  }
  #login-overlay{display:none;position:fixed;inset:0;background:#0d1117cc;backdrop-filter:blur(4px);align-items:center;justify-content:center;z-index:100}
  #login-overlay.show{display:flex}
  #login-box{background:#161b22;border:1px solid #30363d;border-radius:10px;padding:32px;min-width:320px}
  #login-box h2{font-size:1.1rem;margin-bottom:20px}
  #login-box label{display:block;font-size:.8rem;color:#8b949e;margin-bottom:4px}
  #login-box input{width:100%;background:#0d1117;border:1px solid #30363d;color:#e6edf3;padding:8px 10px;border-radius:6px;font-size:.9rem;margin-bottom:14px;outline:none}
  #login-box input:focus{border-color:#58a6ff}
  #login-error{color:#f85149;font-size:.8rem;margin-bottom:10px;min-height:1em}
  #login-box .btn-login{width:100%;background:#238636;border:none;color:#fff;padding:9px;border-radius:6px;font-size:.9rem;cursor:pointer}
  #login-box .btn-login:hover{background:#2ea043}
</style>
</head>
<body>
<div id="login-overlay">
  <div id="login-box">
    <h2>&#x1F512; Sear Root Login</h2>
    <label>Username</label>
    <input id="lu" type="text" value="root" readonly>
    <label>Password</label>
    <input id="lp" type="password" autocomplete="current-password" placeholder="root password">
    <div id="login-error"></div>
    <button class="btn-login" onclick="doLogin()">Sign in</button>
  </div>
</div>
<header>
  <div class="header-brand"><h1>&#x26A1; Sear Daemon</h1></div>
  <nav>
    <a href="/ui">Clients</a>
    <a href="/ui/secrets">Secrets</a>
    <a href="/ui/playbooks">Playbooks</a>
    <a href="/ui/deployments" class="active">Deployments</a>
  </nav>
  <div class="header-right"><button onclick="logout()">Sign out</button></div>
</header>
<div class="container">
  <div class="meta">
    <span id="counts">Loading...</span>
    <span class="spacer"></span>
    <button onclick="load()">Refresh</button>
  </div>
  <div class="filters">
    <input id="f-query" type="text" placeholder="Search deployment, client, playbook">
    <select id="f-status">
      <option value="">All statuses</option>
      <option value="pending">pending</option>
      <option value="running">running</option>
      <option value="rebooting">rebooting</option>
      <option value="done">done</option>
      <option value="failed">failed</option>
    </select>
    <button onclick="clearFilters()">Clear filters</button>
  </div>
  <div id="root"></div>
  <div id="logs-panel" class="panel" style="display:none">
    <div class="panel-head">
      <span id="logs-title">Logs</span>
      <button onclick="closeLogs()">Close</button>
    </div>
    <div id="logs-root" class="log-list"></div>
  </div>
</div>
<script>
function esc(s){var d=document.createElement('div');d.textContent=String(s==null?'':s);return d.innerHTML;}
function authHeader(){var c=sessionStorage.getItem('sear_creds');return c?'Basic '+c:null;}
function showLogin(msg){document.getElementById('login-error').textContent=msg||'';document.getElementById('lp').value='';document.getElementById('login-overlay').classList.add('show');setTimeout(function(){document.getElementById('lp').focus();},50);}
function hideLogin(){document.getElementById('login-overlay').classList.remove('show');}
function logout(){sessionStorage.removeItem('sear_creds');showLogin('');}
async function doLogin(){
  var u=document.getElementById('lu').value;
  var p=document.getElementById('lp').value;
  if(!p){document.getElementById('login-error').textContent='Password required';return;}
  var creds=btoa(u+':'+p);
  var r=await fetch('/deployments',{headers:{Authorization:'Basic '+creds}});
  if(r.status===401){document.getElementById('login-error').textContent='Invalid password';return;}
  sessionStorage.setItem('sear_creds',creds);
  hideLogin();
  load();
}
document.getElementById('lp').addEventListener('keydown',function(e){if(e.key==='Enter')doLogin();});

var deployments=[];

function headersAuth(){var h={};var a=authHeader();if(a)h['Authorization']=a;return h;}

function clearFilters(){document.getElementById('f-query').value='';document.getElementById('f-status').value='';render();}

function filtered(){
  var q=(document.getElementById('f-query').value||'').trim().toLowerCase();
  var s=(document.getElementById('f-status').value||'').trim().toLowerCase();
  return deployments.filter(function(d){
    var status=(d.status||'').toLowerCase();
    var hay=(d.id||'')+' '+(d.client_id||'')+' '+(d.playbook_id||'');
    if(s && status!==s) return false;
    if(q && hay.toLowerCase().indexOf(q)===-1) return false;
    return true;
  });
}

function rowStatus(status){
  var s=(status||'pending').toLowerCase();
  return '<span class="pill '+esc(s)+'"><span class="dot"></span>'+esc(s)+'</span>';
}

function render(){
  var root=document.getElementById('root');
  if(!deployments.length){root.innerHTML='<div class="empty">No deployments yet.</div>';document.getElementById('counts').textContent='Deployments: 0';return;}
  var rowsData=filtered();
  document.getElementById('counts').textContent='Shown: '+rowsData.length+' / '+deployments.length;
  if(!rowsData.length){root.innerHTML='<div class="empty">No deployments match your filters.</div>';return;}
  var rows=rowsData.map(function(d,i){
    var upd=d.updated_at?new Date(d.updated_at).toLocaleString():'-';
    var finished=d.finished_at?new Date(d.finished_at).toLocaleString():'-';
    return '<div class="list-row">'+
      '<div><div>'+esc(d.id||'-')+'</div><div class="mono">resume: '+esc(d.resume_step_index||0)+'</div></div>'+
      '<div class="mono">'+esc(d.client_id||'-')+'</div>'+
      '<div class="mono">'+esc(d.playbook_id||'-')+'</div>'+
      '<div>'+rowStatus(d.status)+'</div>'+
      '<div>'+esc(upd)+'</div>'+
      '<div>'+esc(finished)+'</div>'+
      '<div><button class="logs-btn" data-id="'+encodeURIComponent(String(d.id||''))+'">Logs</button></div>'+
    '</div>';
  }).join('');
  root.innerHTML='<div class="list">'+
    '<div class="list-head"><div>Deployment</div><div>Client</div><div>Playbook</div><div>Status</div><div>Updated</div><div>Finished</div><div></div></div>'+
    rows+
  '</div>';
  var buttons=root.querySelectorAll('.logs-btn');
  buttons.forEach(function(btn){
    btn.addEventListener('click',function(){
      var encodedId=btn.getAttribute('data-id')||'';
      var id='';
      try{id=decodeURIComponent(encodedId);}catch(e){id=encodedId;}
      openLogs(id);
    });
  });
}

async function openLogs(id){
  var p=document.getElementById('logs-panel');
  var root=document.getElementById('logs-root');
  document.getElementById('logs-title').textContent='Logs: '+id;
  p.style.display='block';
  root.innerHTML='<div class="empty">Loading logs...</div>';
  var r=await fetch('/deployments/'+encodeURIComponent(id)+'/logs',{headers:headersAuth()});
  if(r.status===401){showLogin('Session expired - sign in again');return;}
  if(!r.ok){root.innerHTML='<div class="empty">Failed to load logs.</div>';return;}
  var logs=await r.json()||[];
  if(!logs.length){root.innerHTML='<div class="empty">No logs for this deployment.</div>';return;}
  var stepNameByKey={};
  logs.forEach(function(l){
    var m=String(l.message||'').match(/^\[(.+?) \/ (.+?)\] (starting|completed)$/);
    if(m){
      var k=String(l.job_name||m[1]||'')+'|'+String(l.step_index||0);
      stepNameByKey[k]=m[2]||'';
    }
  });
  var rows=logs.map(function(l){
    var ts=l.timestamp?new Date(l.timestamp).toLocaleString():'-';
    var hasStep=!!String(l.job_name||'').trim();
    var job=hasStep?String(l.job_name):'-';
    var step='-';
    if(hasStep){
      var key=job+'|'+String(l.step_index||0);
      step=stepNameByKey[key]||'-';
    }
    return '<div class="log-row">'+
      '<div class="ts">'+esc(ts)+'</div>'+
      '<div>'+esc(l.level||'-')+'</div>'+
      '<div>'+esc(job)+'</div>'+
      '<div>'+esc(step)+'</div>'+
      '<div class="msg">'+esc(l.message||'')+'</div>'+
    '</div>';
  }).join('');
  root.innerHTML='<div class="log-head"><div>Time</div><div>Level</div><div>Job</div><div>Step</div><div>Message</div></div>'+rows;
}
function closeLogs(){document.getElementById('logs-panel').style.display='none';}

async function load(){
  var auth=authHeader();
  if(!auth){showLogin('');return;}
  var r=await fetch('/deployments',{headers:headersAuth()});
  if(r.status===401){showLogin('');return;}
  if(!r.ok){document.getElementById('root').innerHTML='<div class="empty">Failed to load deployments.</div>';return;}
  deployments=await r.json()||[];
  deployments.sort(function(a,b){return new Date(b.updated_at||0)-new Date(a.updated_at||0);});
  render();
}

['f-query','f-status'].forEach(function(id){
  var el=document.getElementById(id);
  el.addEventListener(id==='f-query'?'input':'change',render);
});
load();
setInterval(load,10000);
</script>
</body>
</html>`
