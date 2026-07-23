package webui

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>WhatGate</title>
<style>
  :root { color-scheme: light dark; --bg:#f6f7f9; --card:#fff; --fg:#1a1d21; --muted:#6b7280; --line:#e5e7eb; --accent:#2563eb; --ok:#16a34a; --off:#9ca3af; }
  @media (prefers-color-scheme: dark) {
    :root { --bg:#0f1115; --card:#181b20; --fg:#e6e8eb; --muted:#9aa0a6; --line:#2a2e35; --accent:#3b82f6; }
  }
  * { box-sizing:border-box; }
  body { margin:0; font:15px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif; background:var(--bg); color:var(--fg); }
  .wrap { max-width:720px; margin:0 auto; padding:32px 20px; }
  h1 { font-size:20px; margin:0 0 4px; display:flex; align-items:center; gap:10px; }
  .sub { color:var(--muted); font-size:13px; margin-bottom:24px; }
  .card { background:var(--card); border:1px solid var(--line); border-radius:12px; padding:4px 20px; }
  .row { display:flex; justify-content:space-between; align-items:center; gap:16px; padding:12px 0; border-bottom:1px solid var(--line); }
  .row:last-child { border-bottom:0; }
  .k { color:var(--muted); }
  .v { font-weight:600; text-align:right; word-break:break-all; }
  .mono { font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:13px; }
  .pill { display:inline-block; padding:2px 10px; border-radius:999px; font-size:12px; font-weight:600; }
  .on { background:color-mix(in srgb, var(--ok) 18%, transparent); color:var(--ok); }
  .offp { background:color-mix(in srgb, var(--off) 22%, transparent); color:var(--off); }
  .dot { width:9px; height:9px; border-radius:50%; background:var(--ok); box-shadow:0 0 0 4px color-mix(in srgb,var(--ok) 20%,transparent); }
  .foot { color:var(--muted); font-size:12px; margin-top:16px; text-align:center; }
  input, button { font:inherit; }
  input { padding:6px 10px; border:1px solid var(--line); border-radius:8px; background:var(--bg); color:var(--fg); width:120px; }
  button { padding:6px 14px; border:0; border-radius:8px; background:var(--accent); color:#fff; font-weight:600; cursor:pointer; }
  .msg { color:var(--muted); font-size:12px; padding:0 0 12px; }
</style>
</head>
<body>
<div class="wrap">
  <h1><span class="dot"></span> WhatGate 节点</h1>
  <div class="sub">本地状态面板 · 每 2 秒自动刷新</div>
  <div class="card" id="card">
    <div class="row"><span class="k">加载中…</span><span class="v"></span></div>
  </div>
  <div id="ctrl" style="margin-top:16px"></div>
  <div class="foot" id="foot"></div>
</div>
<script>
const rows = [
  ["Peer ID", s => mono(s.peerID)],
  ["角色", s => s.role || "idle"],
  ["协调器", s => s.coordinator ? mono(s.coordinator) : "（手动/无）"],
  ["作为出口", s => s.exitEnabled ? pill("开启","on") : pill("关闭","offp")],
  ["出口地区", s => s.exitEnabled ? (s.exitRegion || "—") : "—"],
  ["出口负载", s => s.exitEnabled ? String(s.exitLoad) : "—"],
  ["目标地区", s => s.toRegion || "—"],
  ["信任范围", s => s.trustScope || "—"],
  ["已连出口", s => s.connectedExit ? mono(s.connectedExit) : "—"],
  ["SOCKS 入口", s => s.socksAddr || "—"],
  ["运行时长", s => s.uptime || "—"],
];
function esc(x){ return String(x).replace(/[&<>]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[c])); }
function mono(x){ return '<span class="mono">'+esc(x)+'</span>'; }
function pill(t,c){ return '<span class="pill '+c+'">'+esc(t)+'</span>'; }
async function tick(){
  try {
    const s = await (await fetch('api/status',{cache:'no-store'})).json();
    document.getElementById('card').innerHTML = rows.map(([k,f]) =>
      '<div class="row"><span class="k">'+esc(k)+'</span><span class="v">'+f(s)+'</span></div>').join('');
    renderCtrl(s);
    document.getElementById('foot').textContent = '更新于 ' + new Date().toLocaleTimeString();
  } catch(e) {
    document.getElementById('foot').textContent = '无法获取状态：' + e;
  }
}
function renderCtrl(s){
  const ctrl = document.getElementById('ctrl');
  if (!s.canSwitch && !s.canToggleExit) { ctrl.innerHTML = ''; ctrl.dataset.init = ''; return; }
  if (!ctrl.dataset.init) {
    let html = '<div class="card">';
    if (s.canSwitch) html += '<div class="row"><span class="k">切换出口地区</span>'
      + '<span class="v"><input id="rg" placeholder="如 JP"> <button id="sw">切换</button></span></div>';
    if (s.canToggleExit) html += '<div class="row"><span class="k">作为出口</span>'
      + '<span class="v"><button id="ex"></button></span></div>';
    html += '<div class="msg" id="swmsg"></div></div>';
    ctrl.innerHTML = html;
    ctrl.dataset.init = '1';
    if (s.canSwitch) document.getElementById('sw').onclick = doSwitch;
  }
  const ex = document.getElementById('ex');
  if (ex) { ex.textContent = s.exitEnabled ? '关闭出口' : '开启出口'; ex.onclick = () => doToggleExit(!s.exitEnabled); }
}
async function doToggleExit(on){
  const msg = document.getElementById('swmsg');
  msg.textContent = on ? '开启中…' : '关闭中…';
  try {
    const resp = await fetch('api/exit', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({enabled:on})});
    if (!resp.ok) { msg.textContent = '失败：' + esc(await resp.text()); return; }
    msg.textContent = on ? '已开启出口' : '已关闭出口';
    tick();
  } catch(e) { msg.textContent = '错误：' + e; }
}
async function doSwitch(){
  const r = document.getElementById('rg').value.trim();
  const msg = document.getElementById('swmsg');
  if (!r) { msg.textContent = '请输入地区'; return; }
  msg.textContent = '切换中…';
  try {
    const resp = await fetch('api/switch', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({region:r})});
    if (!resp.ok) { msg.textContent = '失败：' + esc(await resp.text()); return; }
    const j = await resp.json();
    msg.textContent = '已切换到出口 ' + esc(j.exit);
    tick();
  } catch(e) { msg.textContent = '错误：' + e; }
}
tick(); setInterval(tick, 2000);
</script>
</body>
</html>`
