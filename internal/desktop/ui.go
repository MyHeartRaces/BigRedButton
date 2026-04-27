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
      --bg: #f6f7f9;
      --panel: #ffffff;
      --text: #16181d;
      --muted: #68707d;
      --border: #d8dde5;
      --accent: #c62828;
      --accent-dark: #991f1f;
      --ok: #176f43;
      --warn: #9a5b00;
      --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #111318;
        --panel: #1a1d24;
        --text: #eef1f5;
        --muted: #a7afbc;
        --border: #333946;
        --accent: #e53935;
        --accent-dark: #b42a27;
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
    h1 {
      font-size: 28px;
      line-height: 1.1;
      margin: 0;
      font-weight: 720;
      letter-spacing: 0;
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
      background: var(--accent);
      color: white;
      min-width: 132px;
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
      .grid { grid-template-columns: 1fr; }
      dl { grid-template-columns: 1fr; }
      dd { margin-bottom: 6px; }
    }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>Big Red Button</h1>
      <div class="status-pill" id="state">loading</div>
    </header>

    <div class="grid">
      <section>
        <h2>Profile</h2>
        <form id="profile-form">
          <label>
            V7 JSON profile
            <input id="profile-file" name="profile" type="file" accept=".json,application/json">
          </label>
          <button type="submit">Save Profile</button>
        </form>
        <div id="profile-summary" style="margin-top: 14px;"></div>
      </section>

      <section>
        <h2>Connection</h2>
        <label>
          Endpoint IP
          <input id="endpoint-ip" autocomplete="off" placeholder="203.0.113.10">
        </label>
        <label>
          WSTunnel binary
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
          ['profile', data.profile.profile],
          ['server', data.profile.server + ':' + data.profile.port],
          ['wstunnel', data.profile.wstunnel_url],
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
        ['profile', data.runtime.active ? data.runtime.active.profile_fingerprint : ''],
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
