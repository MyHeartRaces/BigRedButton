package desktop

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Big Red Button</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #f4f1e8;
      --panel: #fffdf8;
      --text: #172033;
      --muted: #68707d;
      --border: #d9d3c7;
      --accent: #d62828;
      --accent-dark: #a81822;
      --cyan: #0f7890;
      --cream: #fff3cc;
      --ok: #176f43;
      --warn: #9a5b00;
      --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      --display: "Brush Script MT", "Brush Script", "Savoye LET", "Apple Chancery", cursive;
      --block-display: "Phosphate", "Cooper Black", "Rockwell Extra Bold", Georgia, serif;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #10141d;
        --panel: #181d28;
        --text: #eef1f5;
        --muted: #a7afbc;
        --border: #343b4c;
        --accent: #ff4a40;
        --accent-dark: #c92d2c;
        --cyan: #7dd3fc;
        --cream: #f9e4ad;
        --ok: #39b36f;
        --warn: #ffb84d;
      }
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      background: var(--bg);
      color: var(--text);
    }
    main {
      width: min(1100px, calc(100vw - 32px));
      margin: 0 auto;
      padding: 24px 0;
    }
    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 18px;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 16px;
      min-width: 0;
    }
    .brand-mark {
      width: 72px;
      height: 72px;
      flex: 0 0 auto;
    }
    .brand-copy {
      min-width: 0;
    }
    h1 {
      margin: 0;
      letter-spacing: 0;
    }
    .brand-script {
      display: block;
      font-size: 38px;
      line-height: .9;
      color: var(--accent);
      font-family: var(--display);
      font-weight: 700;
      letter-spacing: 0;
      text-shadow: 2px 2px 0 var(--cream), 4px 4px 0 color-mix(in srgb, var(--cyan), transparent 48%);
    }
    .brand-block {
      display: block;
      color: var(--text);
      font-family: var(--block-display);
      font-size: 31px;
      font-weight: 900;
      letter-spacing: 1px;
      line-height: .95;
      margin-top: 3px;
      text-shadow: 1px 1px 0 var(--cream);
    }
    .brand-subtitle {
      color: var(--muted);
      font-size: 13px;
      font-weight: 650;
      letter-spacing: 0;
      margin-top: -2px;
    }
    .status-pill {
      border: 1px solid var(--border);
      border-radius: 999px;
      padding: 7px 12px;
      color: var(--muted);
      background: var(--panel);
      white-space: nowrap;
    }
    .grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 14px;
      align-items: start;
    }
    section {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 16px;
    }
    section.full { grid-column: 1 / -1; }
    h2 {
      font-size: 16px;
      margin: 0 0 12px;
      font-weight: 680;
      letter-spacing: 0;
    }
    label {
      display: grid;
      gap: 6px;
      margin: 12px 0;
      color: var(--muted);
      font-size: 13px;
    }
    input {
      width: 100%;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: transparent;
      color: var(--text);
      min-height: 38px;
      padding: 8px 10px;
      font: inherit;
    }
    .row {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      align-items: center;
    }
    button {
      border: 1px solid var(--border);
      background: var(--panel);
      color: var(--text);
      border-radius: 6px;
      min-height: 38px;
      padding: 8px 12px;
      cursor: pointer;
      font: inherit;
      font-weight: 620;
    }
    button.primary {
      border-color: var(--accent);
      background: linear-gradient(180deg, #f34d45 0%, var(--accent) 54%, var(--accent-dark) 100%);
      color: white;
      min-width: 132px;
      box-shadow: inset 0 1px 0 rgba(255,255,255,.35), 0 2px 0 #650b13;
    }
    button.primary:hover { background: var(--accent-dark); }
    button:disabled {
      opacity: 0.5;
      cursor: default;
    }
    dl {
      display: grid;
      grid-template-columns: minmax(110px, 160px) 1fr;
      gap: 8px 12px;
      margin: 0;
      font-size: 14px;
    }
    dt { color: var(--muted); }
    dd { margin: 0; overflow-wrap: anywhere; }
    .ok { color: var(--ok); }
    .warn { color: var(--warn); }
    pre {
      margin: 0;
      min-height: 140px;
      max-height: 360px;
      overflow: auto;
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 12px;
      background: color-mix(in srgb, var(--panel), #000 5%);
      font: 12px/1.45 var(--mono);
      white-space: pre-wrap;
    }
    @media (max-width: 780px) {
      main { width: min(100vw - 20px, 1100px); padding: 14px 0; }
      header { align-items: flex-start; flex-direction: column; }
      .brand-script { font-size: 34px; }
      .brand-block { font-size: 27px; }
      .brand-mark { width: 62px; height: 62px; }
      .grid { grid-template-columns: 1fr; }
      dl { grid-template-columns: 1fr; }
      dd { margin-bottom: 6px; }
    }
  </style>
</head>
<body>
  <main>
    <header>
      <div class="brand" aria-label="Big Red Button">
        <svg class="brand-mark" viewBox="0 0 96 96" role="img" aria-hidden="true">
          <defs>
            <linearGradient id="miniBg" x1="15" x2="81" y1="14" y2="81" gradientUnits="userSpaceOnUse">
              <stop offset="0" stop-color="#253d65"/>
              <stop offset="1" stop-color="#101827"/>
            </linearGradient>
            <radialGradient id="miniButton" cx="46" cy="37" r="41" gradientUnits="userSpaceOnUse">
              <stop offset="0" stop-color="#ff8277"/>
              <stop offset=".5" stop-color="#df2630"/>
              <stop offset="1" stop-color="#82121c"/>
            </radialGradient>
          </defs>
          <rect width="96" height="96" rx="22" fill="#f9ecd0"/>
          <circle cx="48" cy="48" r="40" fill="#fff2c6"/>
          <circle cx="48" cy="48" r="35" fill="url(#miniBg)" stroke="#0f7890" stroke-width="3"/>
          <path d="M48 15 58 36 82 39 65 55 69 78 48 67 27 78 31 55 14 39 38 36Z" fill="#fff2c6" opacity=".12"/>
          <path d="M20 35c17-11 39-11 56 0M20 61c17 11 39 11 56 0" fill="none" stroke="#82dcf8" stroke-width="3" stroke-linecap="round"/>
          <path d="M19 48h15c7 0 9-7 15-7h28" fill="none" stroke="#82dcf8" stroke-width="3" stroke-linecap="round" opacity=".65"/>
          <ellipse cx="48" cy="66" rx="26" ry="10" fill="#681019"/>
          <circle cx="48" cy="49" r="23" fill="url(#miniButton)" stroke="#5f0b15" stroke-width="3"/>
          <path d="M33 45c4-12 16-18 29-13-4-7-14-10-22-6-8 3-13 11-12 19 0 3 1 6 3 8 1-3 2-6 2-8Z" fill="#ffaaa1" opacity=".62"/>
          <path d="M40 48v-5c0-6 5-11 11-11s11 5 11 11v5h-5v-5c0-3-3-6-6-6s-6 3-6 6v5h-5Z" fill="none" stroke="#fff2c6" stroke-width="4" stroke-linecap="round" stroke-linejoin="round"/>
          <rect x="37" y="46" width="22" height="18" rx="3" fill="#fff2c6"/>
        </svg>
        <div class="brand-copy">
          <h1><span class="brand-script">Big Red</span><span class="brand-block">BUTTON</span></h1>
          <div class="brand-subtitle">VPN launcher</div>
        </div>
      </div>
      <div class="status-pill" id="state">loading</div>
    </header>

    <div class="grid">
      <section>
        <h2>Profile</h2>
        <form id="profile-form">
          <label>
            VPN profile JSON
            <input id="profile-file" name="profile" type="file" accept=".json,application/json">
          </label>
          <button type="submit">Save Profile</button>
        </form>
        <div id="profile-summary" style="margin-top: 14px;"></div>
      </section>

      <section>
        <h2>Connection</h2>
        <label>
          Tunnel gateway IP
          <input id="endpoint-ip" autocomplete="off" placeholder="203.0.113.10">
        </label>
        <label>
          Tunnel helper binary
          <input id="wstunnel-binary" autocomplete="off" placeholder="/usr/bin/wstunnel">
        </label>
        <div class="row">
          <button class="primary" id="connect" type="button">Connect</button>
          <button id="disconnect" type="button">Disconnect</button>
          <button id="refresh" type="button">Refresh</button>
        </div>
      </section>

      <section class="full">
        <h2>Status</h2>
        <div id="runtime"></div>
      </section>

      <section class="full">
        <h2>Output</h2>
        <pre id="output"></pre>
      </section>
    </div>
  </main>

  <script>
    const stateEl = document.getElementById('state');
    const profileSummaryEl = document.getElementById('profile-summary');
    const runtimeEl = document.getElementById('runtime');
    const outputEl = document.getElementById('output');
    const endpointEl = document.getElementById('endpoint-ip');
    const wstunnelEl = document.getElementById('wstunnel-binary');
    const connectButton = document.getElementById('connect');
    const disconnectButton = document.getElementById('disconnect');

    function escapeHTML(value) {
      return String(value ?? '').replace(/[&<>"']/g, char => ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;'
      }[char]));
    }

    function definitionList(items) {
      return '<dl>' + items.map(([key, value]) =>
        '<dt>' + escapeHTML(key) + '</dt><dd>' + escapeHTML(value) + '</dd>'
      ).join('') + '</dl>';
    }

    function setBusy(busy) {
      connectButton.disabled = busy;
      disconnectButton.disabled = busy;
    }

    async function refresh() {
      const response = await fetch('/api/status');
      const data = await response.json();
      stateEl.textContent = data.runtime.state + ' on ' + data.os;
      stateEl.className = 'status-pill ' + (data.runtime.state === 'connected' ? 'ok' : '');

      endpointEl.value = data.gui.endpoint_ip || endpointEl.value || '';
      wstunnelEl.value = data.gui.wstunnel_binary || wstunnelEl.value || '';
      outputEl.textContent = data.gui.last_output || '';

      if (data.profile) {
        profileSummaryEl.innerHTML = definitionList([
          ['server', data.profile.server + ':' + data.profile.port],
          ['gateway', data.profile.wstunnel_url],
          ['addresses', (data.profile.addresses || []).join(', ')],
          ['allowed IPs', (data.profile.allowed_ips || []).join(', ')],
          ['fingerprint', data.profile.fingerprint]
        ]);
      } else {
        profileSummaryEl.innerHTML = '<span class="warn">' + escapeHTML(data.error || 'no profile saved') + '</span>';
      }

      runtimeEl.innerHTML = definitionList([
        ['state', data.runtime.state],
        ['runtime root', data.runtime.runtime_root],
        ['profile fingerprint', data.runtime.active ? data.runtime.active.profile_fingerprint : ''],
        ['interface', data.runtime.active ? data.runtime.active.wireguard_interface : ''],
        ['error', data.runtime.error || '']
      ]);
    }

    document.getElementById('profile-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const file = document.getElementById('profile-file').files[0];
      if (!file) return;
      const form = new FormData();
      form.append('profile', file);
      setBusy(true);
      try {
        const response = await fetch('/api/profile', { method: 'POST', body: form });
        const data = await response.json();
        if (!response.ok) outputEl.textContent = data.error || 'profile upload failed';
        await refresh();
      } finally {
        setBusy(false);
      }
    });

    async function action(path) {
      setBusy(true);
      try {
        const response = await fetch(path, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            endpoint_ip: endpointEl.value,
            wstunnel_binary: wstunnelEl.value
          })
        });
        const data = await response.json();
        outputEl.textContent = data.output || data.error || '';
        await refresh();
      } finally {
        setBusy(false);
      }
    }

    connectButton.addEventListener('click', () => action('/api/connect'));
    disconnectButton.addEventListener('click', () => action('/api/disconnect'));
    document.getElementById('refresh').addEventListener('click', refresh);
    refresh().catch(error => { outputEl.textContent = error.message; });
  </script>
</body>
</html>
`
