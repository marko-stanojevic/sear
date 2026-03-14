package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// HandleSecrets manages the server-side secrets store.
//
//	GET    /secrets          – list secret names (values are never exposed in list)
//	GET    /secrets/{name}   – get a specific secret value
//	PUT    /secrets/{name}   – set or update a secret value
//	DELETE /secrets/{name}   – delete a secret
func (e *Env) HandleSecrets(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets")
	name = strings.TrimPrefix(name, "/")

	switch r.Method {
	case http.MethodGet:
		if name == "" {
			// List names only — never expose values via the list endpoint.
			writeJSON(w, http.StatusOK, e.Store.ListSecretNames())
			return
		}
		val, ok := e.Store.GetSecret(name)
		if !ok {
			writeError(w, http.StatusNotFound, "secret not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"name": name, "value": val})

	case http.MethodPut:
		if name == "" {
			writeError(w, http.StatusBadRequest, "secret name required in path")
			return
		}
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if err := e.Store.SetSecret(name, body.Value); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store secret")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	case http.MethodDelete:
		if name == "" {
			writeError(w, http.StatusBadRequest, "secret name required in path")
			return
		}
		if err := e.Store.DeleteSecret(name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete secret")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleSecretsUI serves the secrets management web page.
func (e *Env) HandleSecretsUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	fmt.Fprint(w, secretsHTML)
}

const secretsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sear — Secrets</title>
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
  .container{max-width:900px;margin:0 auto;padding:24px}
  .meta{color:#8b949e;font-size:.875rem;margin-bottom:20px;display:flex;align-items:center;gap:12px}
  .list{background:#161b22;border:1px solid #30363d;border-radius:8px;overflow:hidden}
  .list-head{display:grid;grid-template-columns:1fr 1.8fr auto;gap:12px;align-items:center;padding:10px 14px;background:#0d1117;color:#8b949e;font-size:.75rem;text-transform:uppercase;letter-spacing:.04em;border-bottom:1px solid #30363d}
  .list-row{display:grid;grid-template-columns:1fr 1.8fr auto;gap:12px;align-items:center;padding:10px 14px;font-size:.875rem;border-bottom:1px solid #21262d}
  .list-row:last-child{border-bottom:none}
  .sname{font-weight:500;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;word-break:break-all}
  .sval{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;color:#8b949e;word-break:break-all;white-space:pre-wrap}
  .sval.revealed{color:#e6edf3}
  .row-actions{display:flex;gap:6px;justify-content:flex-end;white-space:nowrap}
  button{background:#21262d;border:1px solid #30363d;color:#e6edf3;padding:5px 12px;border-radius:6px;cursor:pointer;font-size:.8rem}
  button:hover{background:#30363d}
  .btn-add{background:#238636;border-color:#2ea043;color:#fff}
  .btn-add:hover{background:#2ea043}
  .btn-danger{background:transparent;border-color:transparent;color:#f85149}
  .btn-danger:hover{background:#da363320;border-color:#da3633}
  .btn-muted{background:transparent;border-color:transparent;color:#8b949e}
  .btn-muted:hover{color:#e6edf3;background:#21262d;border-color:#30363d}
  .empty{text-align:center;color:#8b949e;padding:60px 20px}
  .modal-bg{display:none;position:fixed;inset:0;background:#0d1117cc;backdrop-filter:blur(4px);align-items:center;justify-content:center;z-index:50}
  .modal-bg.show{display:flex}
  .modal{background:#161b22;border:1px solid #30363d;border-radius:10px;padding:28px;min-width:380px;max-width:520px;width:90%}
  .modal h3{font-size:1rem;margin-bottom:18px}
  .modal label{display:block;font-size:.8rem;color:#8b949e;margin-bottom:4px}
  .modal input{width:100%;background:#0d1117;border:1px solid #30363d;color:#e6edf3;padding:8px 10px;border-radius:6px;font-size:.875rem;margin-bottom:14px;outline:none;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
  .modal input:focus{border-color:#58a6ff}
  .modal input[readonly]{color:#8b949e}
  .modal-err{color:#f85149;font-size:.8rem;margin-bottom:10px;min-height:1em}
  .modal-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:4px}
  .modal-actions button{padding:7px 18px}
  .val-row{display:flex;align-items:center;gap:6px;margin-bottom:14px}
  .val-row .modal input{margin-bottom:0;flex:1}
  .toggle-vis{background:transparent;border:none;color:#8b949e;cursor:pointer;padding:4px;font-size:.85rem}
  .toggle-vis:hover{color:#e6edf3}
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
<div id="modal-bg" class="modal-bg">
  <div class="modal">
    <h3 id="modal-title">Add Secret</h3>
    <label>Name</label>
    <input id="modal-name" type="text" placeholder="MY_SECRET" spellcheck="false" autocomplete="off">
    <label>Value</label>
    <div style="position:relative;margin-bottom:14px">
      <input id="modal-value" type="password" placeholder="secret value" spellcheck="false" autocomplete="off" style="margin-bottom:0;padding-right:70px">
      <button class="toggle-vis" onclick="toggleModalVis()" style="position:absolute;right:8px;top:50%;transform:translateY(-50%);padding:2px 6px;font-size:.75rem;background:#21262d;border:1px solid #30363d;border-radius:4px;color:#8b949e" id="modal-vis-btn">Show</button>
    </div>
    <div class="modal-err" id="modal-err"></div>
    <div class="modal-actions">
      <button onclick="closeModal()">Cancel</button>
      <button class="btn-add" onclick="saveSecret()">Save</button>
    </div>
  </div>
</div>
<header>
  <div class="header-brand">
    <h1>&#x26A1; Sear Daemon</h1>
  </div>
  <nav>
    <a href="/ui">Clients</a>
    <a href="/ui/secrets" class="active">Secrets</a>
    <a href="/ui/playbooks">Playbooks</a>
    <a href="/ui/deployments">Deployments</a>
  </nav>
  <div class="header-right">
    <button onclick="logout()" style="padding:5px 12px">Sign out</button>
  </div>
</header>
<div class="container">
  <div class="meta">
    <span id="counts">Loading&#x2026;</span>
    <button class="btn-add" onclick="openAdd()">+ Add Secret</button>
  </div>
  <div id="root"></div>
</div>
<script>
function esc(s){var d=document.createElement('div');d.textContent=String(s==null?'':s);return d.innerHTML;}
function authHeader(){var c=sessionStorage.getItem('sear_creds');return c?'Basic '+c:null;}
function showLogin(msg){
  document.getElementById('login-error').textContent=msg||'';
  document.getElementById('lp').value='';
  document.getElementById('login-overlay').classList.add('show');
  setTimeout(function(){document.getElementById('lp').focus();},50);
}
function hideLogin(){document.getElementById('login-overlay').classList.remove('show');}
function logout(){sessionStorage.removeItem('sear_creds');showLogin('');}
async function doLogin(){
  var u=document.getElementById('lu').value;
  var p=document.getElementById('lp').value;
  if(!p){document.getElementById('login-error').textContent='Password required';return;}
  var creds=btoa(u+':'+p);
  var r=await fetch('/api/v1/secrets',{headers:{Authorization:'Basic '+creds}});
  if(r.status===401){document.getElementById('login-error').textContent='Invalid password';return;}
  sessionStorage.setItem('sear_creds',creds);
  hideLogin();
  load();
}
document.getElementById('lp').addEventListener('keydown',function(e){if(e.key==='Enter')doLogin();});

var editingName=null;
var modalVisible=false;

function toggleModalVis(){
  var inp=document.getElementById('modal-value');
  var btn=document.getElementById('modal-vis-btn');
  modalVisible=!modalVisible;
  inp.type=modalVisible?'text':'password';
  btn.textContent=modalVisible?'Hide':'Show';
}
function openAdd(){
  editingName=null;
  document.getElementById('modal-title').textContent='Add Secret';
  document.getElementById('modal-name').value='';
  document.getElementById('modal-name').readOnly=false;
  document.getElementById('modal-value').value='';
  document.getElementById('modal-value').type='password';
  document.getElementById('modal-vis-btn').textContent='Show';
  modalVisible=false;
  document.getElementById('modal-err').textContent='';
  document.getElementById('modal-bg').classList.add('show');
  setTimeout(function(){document.getElementById('modal-name').focus();},50);
}
async function openEdit(i){
  var name=names[i];
  editingName=name;
  document.getElementById('modal-title').textContent='Edit Secret';
  document.getElementById('modal-name').value=name;
  document.getElementById('modal-name').readOnly=true;
  document.getElementById('modal-value').value='';
  document.getElementById('modal-value').type='password';
  document.getElementById('modal-vis-btn').textContent='Show';
  modalVisible=false;
  document.getElementById('modal-err').textContent='';
  document.getElementById('modal-bg').classList.add('show');
  var auth=authHeader();
  var h={};if(auth)h['Authorization']=auth;
  var r=await fetch('/api/v1/secrets/'+encodeURIComponent(name),{headers:h});
  if(r.ok){var d=await r.json();document.getElementById('modal-value').value=d.value||'';}
  setTimeout(function(){document.getElementById('modal-value').focus();},50);
}
function closeModal(){
  document.getElementById('modal-bg').classList.remove('show');
  modalVisible=false;
}
async function saveSecret(){
  var name=document.getElementById('modal-name').value.trim();
  var value=document.getElementById('modal-value').value;
  var errEl=document.getElementById('modal-err');
  if(!name){errEl.textContent='Name is required';return;}
  var auth=authHeader();
  var h={'Content-Type':'application/json'};if(auth)h['Authorization']=auth;
  var r=await fetch('/api/v1/secrets/'+encodeURIComponent(name),{method:'PUT',headers:h,body:JSON.stringify({value:value})});
  if(r.status===401){closeModal();showLogin('Session expired — sign in again');return;}
  if(!r.ok){errEl.textContent='Save failed ('+r.status+')';return;}
  closeModal();
  load();
}
async function deleteSecret(i){
  var name=names[i];
  if(!confirm('Delete secret "'+name+'"?\nThis cannot be undone.'))return;
  var auth=authHeader();
  var h={};if(auth)h['Authorization']=auth;
  var r=await fetch('/api/v1/secrets/'+encodeURIComponent(name),{method:'DELETE',headers:h});
  if(r.status===401){showLogin('Session expired — sign in again');return;}
  if(!r.ok){alert('Delete failed: '+r.status);return;}
  load();
}

var revealed={};
async function toggleReveal(i){
  var name=names[i];
  var el=document.getElementById('v'+i);
  var btn=document.getElementById('b'+i);
  if(revealed[name]!==undefined){
    delete revealed[name];
    el.textContent='&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;';
    el.className='sval';
    btn.textContent='Reveal';
    return;
  }
  var auth=authHeader();
  var h={};if(auth)h['Authorization']=auth;
  var r=await fetch('/api/v1/secrets/'+encodeURIComponent(name),{headers:h});
  if(r.status===401){showLogin('Session expired — sign in again');return;}
  if(!r.ok){alert('Failed to fetch secret value');return;}
  var d=await r.json();
  revealed[name]=d.value||'';
  el.textContent=d.value||'(empty)';
  el.className='sval revealed';
  btn.textContent='Hide';
}

var names=[];
function render(){
  var root=document.getElementById('root');
  if(!names.length){
    root.innerHTML='<div class="empty">No secrets stored yet.</div>';
    document.getElementById('counts').textContent='Secrets: 0';
    return;
  }
  document.getElementById('counts').textContent='Secrets: '+names.length;
  var rows=names.map(function(n,i){
    var isRev=revealed[n]!==undefined;
    var dispVal=isRev?(revealed[n]||'(empty)'):'&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;';
    return '<div class="list-row">'+
      '<div class="sname">'+esc(n)+'</div>'+
      '<div class="sval'+(isRev?' revealed':'')+'" id="v'+i+'">'+dispVal+'</div>'+
      '<div class="row-actions">'+
        '<button class="btn-muted" id="b'+i+'" onclick="toggleReveal('+i+')">'+(isRev?'Hide':'Reveal')+'</button>'+
        '<button onclick="openEdit('+i+')">Edit</button>'+
        '<button class="btn-danger" onclick="deleteSecret('+i+')">Delete</button>'+
      '</div>'+
    '</div>';
  }).join('');
  root.innerHTML='<div class="list">'+
    '<div class="list-head"><div>Name</div><div>Value</div><div></div></div>'+
    rows+
  '</div>';
}
async function load(){
  var auth=authHeader();
  if(!auth){showLogin('');return;}
  var h={};if(auth)h['Authorization']=auth;
  var r=await fetch('/api/v1/secrets',{headers:h});
  if(r.status===401){showLogin('');return;}
  revealed={};
  names=await r.json()||[];
  render();
}
document.getElementById('modal-bg').addEventListener('click',function(e){if(e.target===this)closeModal();});
document.addEventListener('keydown',function(e){if(e.key==='Escape')closeModal();});
document.getElementById('modal-value').addEventListener('keydown',function(e){if(e.key==='Enter')saveSecret();});
load();
</script>
</body>
</html>`
