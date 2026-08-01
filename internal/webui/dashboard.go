package webui

const dashboardHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="theme-color" content="#f6f8fc">
<title>WhatGate · 网络连接</title>
<style>
  :root {
    color-scheme: light;
    --bg:#f5f7fb;
    --surface:#ffffff;
    --surface-soft:#f8fafc;
    --text:#152033;
    --muted:#667085;
    --subtle:#98a2b3;
    --line:#e4e9f1;
    --primary:#3b5bfd;
    --primary-dark:#2f49d1;
    --primary-soft:#eef1ff;
    --success:#16a36a;
    --success-soft:#eaf8f1;
    --warning:#d67817;
    --warning-soft:#fff5e9;
    --danger:#d9485f;
    --danger-soft:#fff0f2;
    --shadow:0 18px 50px rgba(24,39,75,.08);
    --shadow-sm:0 6px 20px rgba(24,39,75,.06);
    --radius:20px;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      color-scheme:dark;
      --bg:#10131a;
      --surface:#171b24;
      --surface-soft:#1d222c;
      --text:#f2f4f7;
      --muted:#aab2c0;
      --subtle:#7b8493;
      --line:#2a313d;
      --primary:#7d8fff;
      --primary-dark:#95a3ff;
      --primary-soft:#252b4a;
      --success:#49c88b;
      --success-soft:#18372b;
      --warning:#f0a14b;
      --warning-soft:#3d2d1b;
      --danger:#f37b8d;
      --danger-soft:#40242b;
      --shadow:0 18px 50px rgba(0,0,0,.25);
      --shadow-sm:0 6px 20px rgba(0,0,0,.2);
    }
  }
  * { box-sizing:border-box; }
  html { scroll-behavior:smooth; }
  body {
    margin:0;
    min-width:320px;
    font:15px/1.55 Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;
    color:var(--text);
    background:
      radial-gradient(circle at 8% -10%, rgba(80,107,255,.12), transparent 28rem),
      radial-gradient(circle at 96% 12%, rgba(51,192,141,.08), transparent 24rem),
      var(--bg);
  }
  button,input,select { font:inherit; }
  button { -webkit-tap-highlight-color:transparent; }
  button:focus-visible,input:focus-visible,select:focus-visible,summary:focus-visible {
    outline:3px solid color-mix(in srgb,var(--primary) 28%,transparent);
    outline-offset:2px;
  }
  [hidden] { display:none !important; }
  .shell { width:min(1120px,calc(100% - 40px)); margin:0 auto; padding-bottom:44px; }
  .topbar { height:82px; display:flex; align-items:center; justify-content:space-between; gap:20px; }
  .brand { display:flex; align-items:center; gap:12px; min-width:0; }
  .brand-mark {
    width:40px; height:40px; display:grid; place-items:center; flex:0 0 auto;
    color:#fff; border-radius:13px;
    background:linear-gradient(145deg,#5270ff,#3048dd);
    box-shadow:0 8px 22px rgba(59,91,253,.24);
  }
  .brand-mark svg { width:24px; height:24px; }
  .brand-name { font-size:18px; font-weight:800; letter-spacing:-.02em; }
  .brand-tagline { color:var(--muted); font-size:12px; margin-top:-2px; }
  .live {
    display:inline-flex; align-items:center; gap:7px; padding:7px 11px;
    color:var(--muted); background:color-mix(in srgb,var(--surface) 80%,transparent);
    border:1px solid var(--line); border-radius:999px; font-size:12px;
  }
  .live-dot { width:7px; height:7px; border-radius:50%; background:var(--success); box-shadow:0 0 0 4px color-mix(in srgb,var(--success) 15%,transparent); }
  .setup {
    min-height:calc(100vh - 126px); display:grid; place-items:center; padding:30px 0 70px;
  }
  .setup-card {
    width:min(760px,100%); background:var(--surface); border:1px solid var(--line);
    border-radius:28px; padding:42px; box-shadow:var(--shadow); position:relative; overflow:hidden;
  }
  .setup-card:before {
    content:""; position:absolute; width:260px; height:260px; border-radius:50%;
    right:-120px; top:-130px; background:var(--primary-soft);
  }
  .step {
    display:inline-flex; align-items:center; gap:8px; padding:6px 10px;
    border-radius:999px; color:var(--primary); background:var(--primary-soft);
    font-weight:750; font-size:12px; position:relative;
  }
  .setup h1 { font-size:32px; line-height:1.2; letter-spacing:-.04em; margin:18px 0 10px; position:relative; }
  .setup-lead { color:var(--muted); margin:0 0 28px; max-width:580px; position:relative; }
  .choice-grid { display:grid; grid-template-columns:1fr 1fr; gap:14px; position:relative; }
  .choice {
    text-align:left; padding:21px; min-height:184px; color:var(--text); background:var(--surface-soft);
    border:1px solid var(--line); border-radius:18px; cursor:pointer; transition:.18s ease;
  }
  .choice:hover { transform:translateY(-2px); border-color:color-mix(in srgb,var(--primary) 45%,var(--line)); box-shadow:var(--shadow-sm); }
  .choice-icon {
    width:42px; height:42px; display:grid; place-items:center; border-radius:13px;
    color:var(--primary); background:var(--primary-soft); margin-bottom:14px;
  }
  .choice-icon svg { width:22px; height:22px; }
  .choice-title { font-size:17px; font-weight:780; display:flex; align-items:center; gap:8px; }
  .recommended { padding:2px 7px; border-radius:999px; color:var(--success); background:var(--success-soft); font-size:10px; font-weight:750; }
  .choice-desc { color:var(--muted); font-size:13px; margin-top:7px; }
  .choice-action { color:var(--primary); font-size:13px; font-weight:750; margin-top:17px; }
  .privacy-note { display:flex; gap:10px; align-items:flex-start; color:var(--muted); font-size:12px; margin-top:20px; }
  .privacy-note svg { width:16px; height:16px; flex:0 0 auto; margin-top:1px; }
  .hero {
    display:grid; grid-template-columns:1.5fr .85fr; gap:20px; padding:34px;
    color:#fff; border-radius:26px; overflow:hidden; position:relative;
    background:linear-gradient(130deg,#243bc5 0%,#3d5eff 55%,#5778ff 100%);
    box-shadow:0 20px 55px rgba(48,72,221,.22);
  }
  .hero:after {
    content:""; position:absolute; right:-90px; top:-155px; width:340px; height:340px;
    border:1px solid rgba(255,255,255,.18); border-radius:50%;
    box-shadow:0 0 0 46px rgba(255,255,255,.045),0 0 0 92px rgba(255,255,255,.03);
  }
  .hero-main,.hero-side { position:relative; z-index:1; }
  .hero-side { display:flex; justify-content:flex-end; align-items:center; }
  .status-badge {
    display:inline-flex; align-items:center; gap:7px; padding:6px 10px;
    border-radius:999px; color:#fff; background:rgba(255,255,255,.15);
    border:1px solid rgba(255,255,255,.2); font-size:12px; font-weight:700;
  }
  .status-badge i { width:7px; height:7px; border-radius:50%; background:#72f2b3; box-shadow:0 0 0 4px rgba(114,242,179,.14); }
  .hero h1 { font-size:30px; line-height:1.22; letter-spacing:-.04em; margin:17px 0 8px; }
  .hero p { margin:0; color:rgba(255,255,255,.78); max-width:590px; }
  .connection-orb {
    width:148px; height:148px; border-radius:50%; display:grid; place-items:center;
    background:rgba(255,255,255,.11); border:1px solid rgba(255,255,255,.18);
    box-shadow:inset 0 0 0 18px rgba(255,255,255,.035);
  }
  .connection-orb svg { width:60px; height:60px; opacity:.95; }
  .content-grid { display:grid; grid-template-columns:minmax(0,1.55fr) minmax(280px,.85fr); gap:20px; margin-top:20px; align-items:start; }
  .stack { display:grid; gap:20px; }
  .card {
    background:var(--surface); border:1px solid var(--line); border-radius:var(--radius);
    padding:24px; box-shadow:var(--shadow-sm);
  }
  .card-head { display:flex; align-items:flex-start; justify-content:space-between; gap:16px; margin-bottom:19px; }
  .card-title { font-size:17px; font-weight:780; letter-spacing:-.02em; margin:0; }
  .card-sub { color:var(--muted); font-size:13px; margin-top:3px; }
  .eyebrow { color:var(--subtle); font-size:11px; font-weight:780; letter-spacing:.08em; text-transform:uppercase; }
  .region-control { display:flex; gap:10px; align-items:end; }
  .field { flex:1; }
  label { display:block; color:var(--muted); font-size:12px; font-weight:700; margin-bottom:7px; }
  select,input {
    width:100%; min-height:46px; padding:10px 13px; color:var(--text); background:var(--surface-soft);
    border:1px solid var(--line); border-radius:12px; transition:.15s ease;
  }
  select:hover,input:hover { border-color:color-mix(in srgb,var(--primary) 38%,var(--line)); }
  .btn {
    min-height:46px; padding:10px 17px; border:0; border-radius:12px; cursor:pointer;
    display:inline-flex; align-items:center; justify-content:center; gap:8px;
    font-weight:750; transition:.15s ease; white-space:nowrap;
  }
  .btn:hover { transform:translateY(-1px); }
  .btn:active { transform:translateY(0); }
  .btn:disabled { opacity:.58; cursor:wait; transform:none; }
  .btn-primary { color:#fff; background:var(--primary); box-shadow:0 8px 18px rgba(59,91,253,.18); }
  .btn-primary:hover { background:var(--primary-dark); }
  .btn-soft { color:var(--primary); background:var(--primary-soft); }
  .btn-danger { color:var(--danger); background:var(--danger-soft); }
  .btn svg { width:17px; height:17px; }
  .proxy-box {
    display:flex; align-items:center; justify-content:space-between; gap:15px;
    padding:15px 16px; border:1px solid var(--line); border-radius:14px; background:var(--surface-soft);
  }
  .proxy-value { font:750 15px ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; word-break:break-all; }
  .proxy-hint { color:var(--muted); font-size:12px; margin-top:3px; }
  .copy-button {
    flex:0 0 auto; border:0; padding:8px 10px; border-radius:10px; cursor:pointer;
    color:var(--primary); background:var(--primary-soft); font-weight:750; font-size:12px;
  }
  .guide {
    display:grid; grid-template-columns:repeat(3,1fr); gap:10px; margin-top:15px;
  }
  .guide-step { padding:13px; border-radius:13px; background:var(--surface-soft); }
  .guide-num {
    width:24px; height:24px; display:grid; place-items:center; margin-bottom:8px;
    border-radius:8px; color:var(--primary); background:var(--primary-soft); font-size:11px; font-weight:800;
  }
  .guide-step b { display:block; font-size:13px; }
  .guide-step span { display:block; color:var(--muted); font-size:11px; margin-top:2px; }
  .status-list { display:grid; gap:1px; }
  .status-row {
    display:flex; justify-content:space-between; gap:15px; align-items:center;
    padding:13px 0; border-bottom:1px solid var(--line);
  }
  .status-row:last-child { border-bottom:0; padding-bottom:0; }
  .status-row:first-child { padding-top:0; }
  .status-key { display:flex; align-items:center; gap:9px; color:var(--muted); }
  .mini-icon { width:30px; height:30px; display:grid; place-items:center; color:var(--primary); background:var(--primary-soft); border-radius:9px; }
  .mini-icon svg { width:16px; height:16px; }
  .status-value { text-align:right; font-weight:740; max-width:58%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .pill { display:inline-flex; align-items:center; padding:4px 9px; border-radius:999px; font-size:11px; font-weight:750; white-space:nowrap; }
  .pill-ok { color:var(--success); background:var(--success-soft); }
  .pill-off { color:var(--muted); background:var(--surface-soft); }
  .share-box { display:flex; align-items:center; justify-content:space-between; gap:16px; }
  .share-copy { min-width:0; }
  .share-copy b { display:block; }
  .share-copy span { display:block; color:var(--muted); font-size:12px; margin-top:4px; }
  .switch {
    width:48px; height:28px; flex:0 0 auto; padding:3px; border:0; border-radius:999px;
    background:var(--line); cursor:pointer; transition:.18s ease;
  }
  .switch:before { content:""; display:block; width:22px; height:22px; border-radius:50%; background:#fff; box-shadow:0 2px 6px rgba(0,0,0,.18); transition:.18s ease; }
  .switch[aria-pressed="true"] { background:var(--success); }
  .switch[aria-pressed="true"]:before { transform:translateX(20px); }
  .warning {
    display:flex; gap:10px; padding:12px; margin-top:14px; border-radius:12px;
    color:var(--warning); background:var(--warning-soft); font-size:12px;
  }
  .warning svg { width:17px; height:17px; flex:0 0 auto; margin-top:1px; }
  details.card { padding:0; overflow:hidden; }
  summary {
    list-style:none; cursor:pointer; padding:20px 24px; display:flex; align-items:center; justify-content:space-between; gap:15px; font-weight:760;
  }
  summary::-webkit-details-marker { display:none; }
  summary:after { content:"›"; color:var(--muted); font-size:24px; line-height:1; transform:rotate(90deg); transition:.15s ease; }
  details[open] summary:after { transform:rotate(-90deg); }
  .details-body { padding:0 24px 24px; border-top:1px solid var(--line); }
  .tech-grid { display:grid; grid-template-columns:1fr 1fr; gap:0 28px; padding-top:10px; }
  .tech-item { padding:11px 0; border-bottom:1px solid var(--line); min-width:0; }
  .tech-item span { display:block; color:var(--muted); font-size:11px; }
  .tech-item b { display:block; margin-top:3px; font-size:13px; word-break:break-all; }
  .group-chips { display:flex; flex-wrap:wrap; gap:7px; margin:12px 0 16px; }
  .group-chip { padding:5px 9px; border-radius:9px; color:var(--primary); background:var(--primary-soft); font-size:12px; font-weight:700; }
  .form-row { display:grid; grid-template-columns:1fr 1fr auto; gap:9px; align-items:end; margin-top:12px; }
  .form-row.endorse { grid-template-columns:1fr 1fr auto; padding-top:15px; border-top:1px solid var(--line); }
  .empty { color:var(--muted); font-size:12px; }
  .footer { display:flex; justify-content:space-between; gap:15px; color:var(--subtle); font-size:11px; padding:19px 4px 0; }
  .skeleton { overflow:hidden; position:relative; background:rgba(255,255,255,.12); border-radius:7px; color:transparent !important; }
  .skeleton:after { content:""; position:absolute; inset:0; transform:translateX(-100%); background:linear-gradient(90deg,transparent,rgba(255,255,255,.18),transparent); animation:shine 1.3s infinite; }
  @keyframes shine { to { transform:translateX(100%); } }
  .toast {
    position:fixed; left:50%; bottom:26px; z-index:20; min-width:260px; max-width:min(520px,calc(100% - 30px));
    transform:translate(-50%,18px); opacity:0; pointer-events:none; padding:12px 16px;
    color:#fff; background:#1e293b; border-radius:12px; box-shadow:0 12px 35px rgba(0,0,0,.22);
    text-align:center; font-size:13px; transition:.2s ease;
  }
  .toast.show { transform:translate(-50%,0); opacity:1; }
  .toast.error { background:#b4233a; }
  .spinner { width:15px; height:15px; border:2px solid currentColor; border-right-color:transparent; border-radius:50%; animation:spin .65s linear infinite; }
  @keyframes spin { to { transform:rotate(360deg); } }
  @media (max-width:820px) {
    .hero { grid-template-columns:1fr; }
    .hero-side { display:none; }
    .content-grid { grid-template-columns:1fr; }
    .side-stack { grid-template-columns:1fr 1fr; }
  }
  @media (max-width:620px) {
    .shell { width:min(100% - 24px,1120px); }
    .topbar { height:70px; }
    .brand-tagline,.live span { display:none; }
    .setup { min-height:calc(100vh - 94px); padding:16px 0 40px; align-items:start; }
    .setup-card { padding:27px 20px; border-radius:22px; }
    .setup h1 { font-size:26px; }
    .choice-grid { grid-template-columns:1fr; }
    .choice { min-height:auto; padding:18px; }
    .hero { padding:25px 22px; border-radius:21px; }
    .hero h1 { font-size:25px; }
    .content-grid { margin-top:12px; gap:12px; }
    .stack,.side-stack { gap:12px; grid-template-columns:1fr; }
    .card { padding:20px; border-radius:17px; }
    .region-control { align-items:stretch; flex-direction:column; }
    .btn { width:100%; }
    .guide { grid-template-columns:1fr; }
    .guide-step { display:grid; grid-template-columns:28px 1fr; column-gap:8px; }
    .guide-num { grid-row:1 / span 2; margin:0; }
    .proxy-box { align-items:flex-start; }
    .tech-grid { grid-template-columns:1fr; }
    .form-row,.form-row.endorse { grid-template-columns:1fr; }
    .footer { flex-direction:column; text-align:center; }
  }
</style>
</head>
<body>
<div class="shell">
  <header class="topbar">
    <div class="brand">
      <div class="brand-mark" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none"><path d="M5 9.5V8a7 7 0 0 1 14 0v1.5M4 9.5h16v8.2a2.3 2.3 0 0 1-2.3 2.3H6.3A2.3 2.3 0 0 1 4 17.7V9.5Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/><path d="m9.2 14.3 1.8 1.8 3.9-4.2" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </div>
      <div>
        <div class="brand-name">WhatGate</div>
        <div class="brand-tagline">可信的伙伴网络</div>
      </div>
    </div>
    <div class="live"><i class="live-dot"></i><span>本机服务运行中</span></div>
  </header>

  <main id="loadingView" class="hero" aria-live="polite">
    <div class="hero-main">
      <div class="status-badge"><i></i>正在读取状态</div>
      <h1 class="skeleton" style="width:58%">正在准备…</h1>
      <p class="skeleton" style="width:72%">加载本机连接信息</p>
    </div>
  </main>

  <main id="setupView" class="setup" hidden>
    <section class="setup-card">
      <div class="step">第 1 步，共 1 步</div>
      <h1>选择适合你的连接方式</h1>
      <p class="setup-lead">这决定你愿意连接哪些伙伴提供的网络。以后仍可通过重启并修改设置来更换。</p>
      <div class="choice-grid">
        <button class="choice" data-scope="conservative">
          <span class="choice-icon">
            <svg viewBox="0 0 24 24" fill="none"><path d="M12 3 5 6v5c0 4.6 2.8 8.3 7 10 4.2-1.7 7-5.4 7-10V6l-7-3Z" stroke="currentColor" stroke-width="1.8"/><path d="m9 12 2 2 4-4" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>
          </span>
          <span class="choice-title">安全优先 <span class="recommended">推荐</span></span>
          <span class="choice-desc">只连接自己信任圈内的伙伴。选择可能较少，但最适合日常使用。</span>
          <span class="choice-action">使用安全模式 →</span>
        </button>
        <button class="choice" data-scope="open">
          <span class="choice-icon">
            <svg viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="1.8"/><path d="M3.5 12h17M12 3c2.3 2.5 3.5 5.5 3.5 9S14.3 18.5 12 21c-2.3-2.5-3.5-5.5-3.5-9S9.7 5.5 12 3Z" stroke="currentColor" stroke-width="1.8"/></svg>
          </span>
          <span class="choice-title">可用优先</span>
          <span class="choice-desc">也可以连接更广泛网络中的出口，可选地区更多，但提供者可能并不认识。</span>
          <span class="choice-action">使用开放模式 →</span>
        </button>
      </div>
      <div class="privacy-note">
        <svg viewBox="0 0 24 24" fill="none"><path d="M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Z" stroke="currentColor" stroke-width="1.8"/><path d="M12 11v6M12 7.5h.01" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
        <span>你的网络内容不会经过协调服务；数据会加密后直接发送给所选伙伴。</span>
      </div>
    </section>
  </main>

  <main id="appView" hidden>
    <section class="hero">
      <div class="hero-main">
        <div class="status-badge"><i id="statusDot"></i><span id="statusLabel">检查中</span></div>
        <h1 id="heroTitle">正在检查连接…</h1>
        <p id="heroText">正在读取本机节点状态。</p>
      </div>
      <div class="hero-side" aria-hidden="true">
        <div class="connection-orb">
          <svg viewBox="0 0 64 64" fill="none"><path d="M19.5 36.5 11 45a8.5 8.5 0 0 0 12 12l9.7-9.7M44.5 27.5 53 19A8.5 8.5 0 1 0 41 7l-9.7 9.7" stroke="currentColor" stroke-width="4" stroke-linecap="round"/><path d="m23 41 18-18" stroke="currentColor" stroke-width="4" stroke-linecap="round"/></svg>
        </div>
      </div>
    </section>

    <div class="content-grid">
      <div class="stack">
        <section class="card" id="regionCard" hidden>
          <div class="card-head">
            <div>
              <h2 class="card-title">选择访问地区</h2>
              <div class="card-sub">切换后，新打开的连接会使用新的伙伴网络</div>
            </div>
            <span id="regionPill" class="pill pill-ok">已连接</span>
          </div>
          <div class="region-control">
            <div class="field">
              <label for="regionSelect">目标地区</label>
              <select id="regionSelect">
                <option value="JP">日本</option>
                <option value="US">美国</option>
                <option value="SG">新加坡</option>
                <option value="HK">中国香港</option>
                <option value="GB">英国</option>
                <option value="DE">德国</option>
                <option value="AU">澳大利亚</option>
                <option value="CA">加拿大</option>
                <option value="OTHER">其他地区…</option>
              </select>
            </div>
            <div class="field" id="customRegionField" hidden>
              <label for="customRegion">地区代码</label>
              <input id="customRegion" maxlength="12" autocomplete="off" placeholder="例如 FR">
            </div>
            <button class="btn btn-primary" id="switchButton">
              <svg viewBox="0 0 24 24" fill="none"><path d="M7 7h11l-3-3M17 17H6l3 3" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>
              连接此地区
            </button>
          </div>
        </section>

        <section class="card" id="useCard" hidden>
          <div class="card-head">
            <div>
              <div class="eyebrow">开始使用</div>
              <h2 class="card-title">在应用中填入这个代理地址</h2>
              <div class="card-sub">只需设置一次；请选择 SOCKS5，并开启“通过代理解析域名”</div>
            </div>
          </div>
          <div class="proxy-box">
            <div>
              <div class="proxy-value" id="proxyAddress">—</div>
              <div class="proxy-hint">SOCKS5 · 仅本机可访问</div>
            </div>
            <button class="copy-button" id="copyProxy">复制地址</button>
          </div>
          <div class="guide">
            <div class="guide-step"><span class="guide-num">1</span><b>打开应用设置</b><span>找到网络或代理设置</span></div>
            <div class="guide-step"><span class="guide-num">2</span><b>选择 SOCKS5</b><span>填入上方地址和端口</span></div>
            <div class="guide-step"><span class="guide-num">3</span><b>开启远程 DNS</b><span>避免域名在本地泄露</span></div>
          </div>
        </section>

        <details class="card" id="groupsCard" hidden>
          <summary>管理我的信任圈</summary>
          <div class="details-body">
            <div class="card-sub">信任圈用于限定你愿意连接或服务的伙伴。</div>
            <div class="group-chips" id="groupChips"></div>
            <form class="form-row" id="joinForm">
              <div class="field"><label for="groupID">信任圈名称</label><input id="groupID" required autocomplete="off" placeholder="例如 family"></div>
              <div class="field"><label for="groupSecret">加入口令</label><input id="groupSecret" type="password" autocomplete="new-password" placeholder="由创建者提供"></div>
              <button class="btn btn-soft" type="submit">加入</button>
            </form>
            <form class="form-row endorse" id="endorseForm">
              <div class="field"><label for="fromGroup">我的信任圈</label><input id="fromGroup" required autocomplete="off"></div>
              <div class="field"><label for="toGroup">要信任的圈</label><input id="toGroup" required autocomplete="off"></div>
              <button class="btn btn-soft" type="submit">确认信任</button>
            </form>
          </div>
        </details>
      </div>

      <div class="stack side-stack">
        <section class="card">
          <div class="card-head">
            <div>
              <h2 class="card-title">运行状态</h2>
              <div class="card-sub">页面会自动更新</div>
            </div>
          </div>
          <div class="status-list">
            <div class="status-row">
              <span class="status-key"><i class="mini-icon"><svg viewBox="0 0 24 24" fill="none"><path d="M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z" stroke="currentColor" stroke-width="1.8"/><path d="M3.5 12h17M12 3c2 2.4 3.1 5.4 3.1 9S14 18.6 12 21c-2-2.4-3.1-5.4-3.1-9S10 5.4 12 3Z" stroke="currentColor" stroke-width="1.8"/></svg></i>当前地区</span>
              <span class="status-value" id="currentRegion">—</span>
            </div>
            <div class="status-row">
              <span class="status-key"><i class="mini-icon"><svg viewBox="0 0 24 24" fill="none"><path d="M12 7v5l3 2M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg></i>运行时间</span>
              <span class="status-value" id="uptime">—</span>
            </div>
            <div class="status-row">
              <span class="status-key"><i class="mini-icon"><svg viewBox="0 0 24 24" fill="none"><path d="M7 12h10M12 7v10M5 20h14a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg></i>使用模式</span>
              <span class="status-value" id="trustMode">—</span>
            </div>
          </div>
        </section>

        <section class="card" id="shareCard" hidden>
          <div class="share-box">
            <div class="share-copy">
              <b>帮助伙伴访问网络</b>
              <span id="shareDescription">当前没有共享本机网络</span>
            </div>
            <button class="switch" id="shareSwitch" role="switch" aria-label="共享本机网络" aria-pressed="false"></button>
          </div>
          <div class="warning" id="shareWarning" hidden>
            <svg viewBox="0 0 24 24" fill="none"><path d="M12 9v4M12 17h.01M10.3 4.4 2.6 18a2 2 0 0 0 1.8 3h15.2a2 2 0 0 0 1.8-3L13.7 4.4a2 2 0 0 0-3.4 0Z" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>
            <span>开启后，伙伴访问的网站会看到你的公网 IP。请确认你理解并接受这一点。</span>
          </div>
        </section>

        <details class="card">
          <summary>高级信息</summary>
          <div class="details-body">
            <div class="tech-grid">
              <div class="tech-item"><span>节点编号</span><b id="peerID">—</b></div>
              <div class="tech-item"><span>当前角色</span><b id="role">—</b></div>
              <div class="tech-item"><span>协调服务</span><b id="coordinator">—</b></div>
              <div class="tech-item"><span>已连伙伴</span><b id="connectedExit">—</b></div>
              <div class="tech-item"><span>共享地区</span><b id="exitRegion">—</b></div>
              <div class="tech-item"><span>共享负载</span><b id="exitLoad">—</b></div>
            </div>
          </div>
        </details>
      </div>
    </div>

    <footer class="footer">
      <span>WhatGate 仅在本机提供此控制页面</span>
      <span id="lastUpdated">等待更新</span>
    </footer>
  </main>
</div>
<div class="toast" id="toast" role="status" aria-live="polite"></div>
<script>
const regions = {
  JP:"日本", US:"美国", SG:"新加坡", HK:"中国香港",
  GB:"英国", DE:"德国", AU:"澳大利亚", CA:"加拿大"
};
const icons = {
  spinner:'<span class="spinner"></span>'
};
let state = null;
let toastTimer = null;
let busy = false;

function byID(id){ return document.getElementById(id); }
function text(id,value){ byID(id).textContent = value == null || value === "" ? "—" : String(value); }
function regionName(code){ return regions[code] || code || "尚未选择"; }
function roleName(role){
  return ({client:"使用网络",exit:"分享网络","client+exit":"使用并分享",idle:"待机"})[role] || "待机";
}
function trustName(scope){
  return ({conservative:"安全优先",open:"可用优先"})[scope] || "尚未选择";
}
function isClient(s){ return s.role === "client" || s.role === "client+exit"; }
function friendlyError(error){
  const message = error && error.message ? error.message : String(error || "");
  if (message.includes("no exit in region")) return "该地区暂时没有可用伙伴，请稍后再试";
  if (message.includes("choose a trust scope first")) return "请先完成首次连接设置";
  if (message.includes("no exit selected yet")) return "尚未连接到可用伙伴";
  if (message.includes("Failed to fetch") || message.includes("NetworkError")) return "本机服务暂时没有响应";
  return message || "服务暂时不可用";
}
function showToast(message,isError){
  const el = byID("toast");
  clearTimeout(toastTimer);
  el.textContent = message;
  el.className = "toast show" + (isError ? " error" : "");
  toastTimer = setTimeout(function(){ el.className = "toast"; },3200);
}
async function request(path,options){
  const response = await fetch(path,options);
  if (!response.ok) {
    const detail = (await response.text()).trim();
    throw new Error(detail || "服务暂时不可用");
  }
  const type = response.headers.get("content-type") || "";
  return type.includes("application/json") ? response.json() : null;
}
function setButtonBusy(button,on,label){
  if (!button) return;
  if (on) {
    button.dataset.label = button.innerHTML;
    button.innerHTML = icons.spinner + label;
    button.disabled = true;
  } else {
    button.innerHTML = button.dataset.label || button.innerHTML;
    button.disabled = false;
  }
}
function selectCurrentRegion(code){
  const select = byID("regionSelect");
  const known = Object.prototype.hasOwnProperty.call(regions,code);
  select.value = known ? code : "OTHER";
  byID("customRegionField").hidden = known;
  if (!known && code) byID("customRegion").value = code;
}
function renderSetup(){
  byID("loadingView").hidden = true;
  byID("appView").hidden = true;
  byID("setupView").hidden = false;
}
function renderApp(s){
  state = s;
  byID("loadingView").hidden = true;
  byID("setupView").hidden = true;
  byID("appView").hidden = false;

  const client = isClient(s);
  const connected = Boolean(s.connectedExit);
  let label = "节点已启动";
  let title = "WhatGate 已准备好";
  let description = "当前处于待机状态。";
  if (connected) {
    label = "安全连接已就绪";
    title = "已连接到" + regionName(s.toRegion);
    description = "通道已经建立。按照下方提示设置应用代理后即可使用。";
  } else if (client) {
    label = "正在等待连接";
    title = "暂未找到可用伙伴";
    description = "可以重新选择地区并尝试连接；已有的本机代理会继续等待。";
  } else if (s.exitEnabled) {
    label = "正在帮助伙伴";
    title = "本机网络正在共享";
    description = "WhatGate 正在按你的安全规则为信任网络提供出口。";
  }
  text("statusLabel",label);
  text("heroTitle",title);
  text("heroText",description);

  byID("regionCard").hidden = !s.canSwitch;
  byID("useCard").hidden = !s.socksAddr;
  byID("groupsCard").hidden = !s.canManage;
  byID("shareCard").hidden = !s.canToggleExit;
  if (s.canSwitch) {
    const editingRegion = byID("regionSelect").matches(":focus") || byID("customRegion").matches(":focus");
    if (!editingRegion && !busy) selectCurrentRegion(s.toRegion);
    const pill = byID("regionPill");
    pill.textContent = connected ? "已连接" : "未连接";
    pill.className = "pill " + (connected ? "pill-ok" : "pill-off");
  }
  text("proxyAddress",s.socksAddr);
  text("currentRegion",client ? regionName(s.toRegion) : (s.exitEnabled ? regionName(s.exitRegion) : "待机"));
  text("uptime",s.uptime);
  text("trustMode",trustName(s.trustScope));
  text("peerID",s.peerID);
  text("role",roleName(s.role));
  text("coordinator",s.coordinator || "手动连接 / 未设置");
  text("connectedExit",s.connectedExit || "尚未连接");
  text("exitRegion",s.exitEnabled ? regionName(s.exitRegion) : "未共享");
  text("exitLoad",s.exitEnabled ? s.exitLoad : "—");

  const shareSwitch = byID("shareSwitch");
  shareSwitch.setAttribute("aria-pressed",String(Boolean(s.exitEnabled)));
  shareSwitch.setAttribute("aria-label",s.exitEnabled ? "停止共享本机网络" : "共享本机网络");
  text("shareDescription",s.exitEnabled ? "正在为伙伴提供网络出口" : "默认关闭，不会占用你的网络");
  byID("shareWarning").hidden = !s.exitEnabled;
  renderGroups(s.groups || []);
  text("lastUpdated","刚刚更新 · " + new Date().toLocaleTimeString([], {hour:"2-digit",minute:"2-digit",second:"2-digit"}));
}
function renderGroups(groups){
  const box = byID("groupChips");
  if (!groups.length) {
    box.innerHTML = '<span class="empty">尚未加入任何信任圈</span>';
    return;
  }
  box.innerHTML = "";
  groups.forEach(function(group){
    const chip = document.createElement("span");
    chip.className = "group-chip";
    chip.textContent = group;
    box.appendChild(chip);
  });
  if (!byID("fromGroup").value) byID("fromGroup").value = groups[0];
}
async function refresh(){
  try {
    const next = await request("api/status",{cache:"no-store"});
    if (next.needsSetup) renderSetup();
    else renderApp(next);
  } catch (error) {
    byID("loadingView").hidden = false;
    byID("setupView").hidden = true;
    byID("appView").hidden = true;
    byID("loadingView").querySelector(".status-badge").lastChild.textContent = "无法读取本机服务";
    byID("loadingView").querySelector("h1").className = "";
    byID("loadingView").querySelector("h1").textContent = "连接页面暂时不可用";
    byID("loadingView").querySelector("p").className = "";
    byID("loadingView").querySelector("p").textContent = "请确认 WhatGate 仍在运行，然后刷新此页面。";
  }
}
document.querySelectorAll("[data-scope]").forEach(function(button){
  button.addEventListener("click",async function(){
    if (busy) return;
    busy = true;
    const buttons = document.querySelectorAll("[data-scope]");
    buttons.forEach(function(item){ item.disabled = true; });
    const original = button.querySelector(".choice-action").textContent;
    button.querySelector(".choice-action").innerHTML = icons.spinner + " 正在寻找可用伙伴…";
    try {
      await request("api/setup",{
        method:"POST",
        headers:{"Content-Type":"application/json"},
        body:JSON.stringify({scope:button.dataset.scope})
      });
      showToast("设置完成，连接已建立");
      await refresh();
    } catch (error) {
      button.querySelector(".choice-action").textContent = original;
      buttons.forEach(function(item){ item.disabled = false; });
      showToast("暂时无法完成设置：" + friendlyError(error),true);
    } finally {
      busy = false;
    }
  });
});
byID("regionSelect").addEventListener("change",function(){
  byID("customRegionField").hidden = this.value !== "OTHER";
  if (this.value === "OTHER") byID("customRegion").focus();
});
byID("switchButton").addEventListener("click",async function(){
  if (busy) return;
  const select = byID("regionSelect");
  const region = (select.value === "OTHER" ? byID("customRegion").value : select.value).trim().toUpperCase();
  if (!region) {
    showToast("请输入地区代码，例如 FR",true);
    return;
  }
  busy = true;
  setButtonBusy(this,true,"连接中…");
  try {
    await request("api/switch",{
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({region:region})
    });
    showToast("已连接到" + regionName(region));
    await refresh();
  } catch (error) {
    showToast("切换失败：" + friendlyError(error),true);
  } finally {
    setButtonBusy(this,false);
    busy = false;
  }
});
byID("copyProxy").addEventListener("click",async function(){
  const value = byID("proxyAddress").textContent;
  try {
    await navigator.clipboard.writeText(value);
    showToast("代理地址已复制");
  } catch (_) {
    const range = document.createRange();
    range.selectNodeContents(byID("proxyAddress"));
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    showToast("已选中地址，请手动复制");
  }
});
byID("shareSwitch").addEventListener("click",async function(){
  if (busy || !state) return;
  const enable = !state.exitEnabled;
  if (enable && !window.confirm("开启后，伙伴访问的网站会看到你的公网 IP。确认要共享本机网络吗？")) return;
  busy = true;
  this.disabled = true;
  try {
    await request("api/exit",{
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({enabled:enable})
    });
    showToast(enable ? "已开始帮助伙伴" : "已停止共享本机网络");
    await refresh();
  } catch (error) {
    showToast("操作失败：" + friendlyError(error),true);
  } finally {
    this.disabled = false;
    busy = false;
  }
});
byID("joinForm").addEventListener("submit",async function(event){
  event.preventDefault();
  if (busy) return;
  const button = this.querySelector("button");
  const groupID = byID("groupID").value.trim();
  busy = true;
  setButtonBusy(button,true,"加入中");
  try {
    await request("api/group/join",{
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({groupID:groupID,secret:byID("groupSecret").value})
    });
    byID("groupSecret").value = "";
    showToast("已加入信任圈 " + groupID);
    await refresh();
  } catch (error) {
    showToast("加入失败：" + friendlyError(error),true);
  } finally {
    setButtonBusy(button,false);
    busy = false;
  }
});
byID("endorseForm").addEventListener("submit",async function(event){
  event.preventDefault();
  if (busy) return;
  const button = this.querySelector("button");
  const from = byID("fromGroup").value.trim();
  const to = byID("toGroup").value.trim();
  busy = true;
  setButtonBusy(button,true,"确认中");
  try {
    await request("api/group/endorse",{
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({fromGroup:from,toGroup:to})
    });
    showToast("已建立 " + from + " 与 " + to + " 的信任");
  } catch (error) {
    showToast("设置失败：" + friendlyError(error),true);
  } finally {
    setButtonBusy(button,false);
    busy = false;
  }
});

refresh();
setInterval(function(){ if (!busy) refresh(); },3000);
</script>
</body>
</html>`
