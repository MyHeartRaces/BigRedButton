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
    button.danger {
      border-color: var(--accent-dark);
      color: var(--accent-dark);
    }
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
          Tunnel gateway IP override
          <input id="endpoint-ip" autocomplete="off" placeholder="optional resolved IP">
        </label>
        <label>
          Tunnel helper binary
          <input id="wstunnel-binary" autocomplete="off" placeholder="/usr/bin/wstunnel">
        </label>
        <div class="row">
          <button class="primary" id="connect" type="button">Connect</button>
          <button id="disconnect" type="button">Disconnect</button>
          <button id="preflight" type="button">Preflight</button>
          <button id="diagnostics" type="button">Diagnostics</button>
          <button id="diagnostics-bundle" type="button">Bundle</button>
          <button id="refresh" type="button">Refresh</button>
        </div>
      </section>

      <section>
        <h2>Isolated App</h2>
        <label>
          Session UUID
          <input id="isolated-session" autocomplete="off" placeholder="auto-generated">
        </label>
        <label>
          App command
          <input id="isolated-command" autocomplete="off" placeholder="/usr/bin/curl https://example.com">
        </label>
        <div class="row">
          <button id="isolated-preflight" type="button">Preflight</button>
          <button class="primary" id="isolated-start" type="button">Start App</button>
          <button id="isolated-stop" type="button">Stop App</button>
          <button class="danger" id="isolated-cleanup" type="button">Cleanup</button>
          <button class="danger" id="isolated-recover" type="button">Recover Dirty</button>
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
    (function () {
      'use strict';

      var stateEl = document.getElementById('state');
      var profileSummaryEl = document.getElementById('profile-summary');
      var runtimeEl = document.getElementById('runtime');
      var outputEl = document.getElementById('output');
      var profileFileEl = document.getElementById('profile-file');
      var endpointEl = document.getElementById('endpoint-ip');
      var wstunnelEl = document.getElementById('wstunnel-binary');
      var isolatedSessionEl = document.getElementById('isolated-session');
      var isolatedCommandEl = document.getElementById('isolated-command');
      var connectButton = document.getElementById('connect');
      var disconnectButton = document.getElementById('disconnect');
      var preflightButton = document.getElementById('preflight');
      var diagnosticsButton = document.getElementById('diagnostics');
      var diagnosticsBundleButton = document.getElementById('diagnostics-bundle');
      var refreshButton = document.getElementById('refresh');
      var isolatedPreflightButton = document.getElementById('isolated-preflight');
      var isolatedStartButton = document.getElementById('isolated-start');
      var isolatedStopButton = document.getElementById('isolated-stop');
      var isolatedCleanupButton = document.getElementById('isolated-cleanup');
      var isolatedRecoverButton = document.getElementById('isolated-recover');
      var actionButtons = [
        connectButton,
        disconnectButton,
        preflightButton,
        diagnosticsButton,
        diagnosticsBundleButton,
        refreshButton,
        isolatedPreflightButton,
        isolatedStartButton,
        isolatedStopButton,
        isolatedCleanupButton,
        isolatedRecoverButton
      ];
      var currentSystemState = 'Idle';
      var lastRenderedOutput = '';

      function text(value) {
        return String(value == null ? '' : value);
      }

      function escapeHTML(value) {
        return text(value).replace(/[&<>"']/g, function (char) {
          var map = {
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#39;'
          };
          return map[char];
        });
      }

      function join(values) {
        return values && values.length ? values.join(', ') : '';
      }

      function definitionList(items) {
        var html = '<dl>';
        for (var index = 0; index < items.length; index += 1) {
          html += '<dt>' + escapeHTML(items[index][0]) + '</dt><dd>' + escapeHTML(items[index][1]) + '</dd>';
        }
        return html + '</dl>';
      }

      function setOutput(message) {
        lastRenderedOutput = text(message);
        outputEl.textContent = lastRenderedOutput;
      }

      function describeError(error) {
        if (!error) return 'unknown error';
        if (error.message) return error.message;
        return text(error);
      }

      function setBusy(busy, message) {
        for (var index = 0; index < actionButtons.length; index += 1) {
          if (actionButtons[index]) actionButtons[index].disabled = busy;
        }
        if (busy && message) {
          stateEl.textContent = message;
          setOutput(message);
        }
      }

      function requestJSON(method, path, body, headers) {
        return new Promise(function (resolve, reject) {
          if (!window.XMLHttpRequest) {
            reject(new Error('this browser does not support XMLHttpRequest'));
            return;
          }
          var xhr = new XMLHttpRequest();
          xhr.open(method, path, true);
          xhr.timeout = 180000;
          xhr.setRequestHeader('Accept', 'application/json');
          if (headers) {
            for (var key in headers) {
              if (Object.prototype.hasOwnProperty.call(headers, key)) {
                xhr.setRequestHeader(key, headers[key]);
              }
            }
          }
          xhr.onreadystatechange = function () {
            if (xhr.readyState !== 4) return;
            var raw = xhr.responseText || '';
            var data = {};
            if (raw) {
              try {
                data = JSON.parse(raw);
              } catch (parseError) {
                reject(new Error('HTTP ' + xhr.status + ' ' + xhr.statusText + '\n' + raw));
                return;
              }
            }
            resolve({
              ok: xhr.status >= 200 && xhr.status < 300,
              status: xhr.status,
              statusText: xhr.statusText,
              data: data || {}
            });
          };
          xhr.onerror = function () {
            reject(new Error('network error while calling ' + path));
          };
          xhr.ontimeout = function () {
            reject(new Error('timeout while calling ' + path));
          };
          xhr.send(body || null);
        });
      }

      function responseMessage(result, fallback) {
        var data = result && result.data ? result.data : {};
        return data.output || data.error || fallback || '';
      }

      function failedResponseMessage(result, fallback) {
        var message = responseMessage(result, '');
        if (message) return message;
        if (result) return 'HTTP ' + result.status + ' ' + result.statusText;
        return fallback || 'request failed';
      }

      function activeIsolatedSession(data, sessions) {
        if (data.isolated && data.isolated.active) return data.isolated.active;
        for (var index = 0; index < sessions.length; index += 1) {
          var snapshot = sessions[index].snapshot || {};
          if (snapshot.active && snapshot.state === 'Connected') return snapshot.active;
        }
        return null;
      }

      function hasSessionState(data, sessions, state) {
        if (data.isolated && data.isolated.state === state) return true;
        for (var index = 0; index < sessions.length; index += 1) {
          var snapshot = sessions[index].snapshot || {};
          if (snapshot.state === state) return true;
        }
        return false;
      }

      function knownSessionsText(sessions) {
        var values = [];
        for (var index = 0; index < sessions.length; index += 1) {
          var snapshot = sessions[index].snapshot || {};
          var active = snapshot.active || {};
          values.push(text(sessions[index].session_id) + ' ' + text(snapshot.state) + (active.namespace ? ' ' + active.namespace : ''));
        }
        return values.join(', ');
      }

      function render(data) {
        data = data || {};
        var runtime = data.runtime || {};
        var gui = data.gui || {};
        var sessions = data.isolated_sessions || [];
        var isolated = activeIsolatedSession(data, sessions);
        var connected = isolated || hasSessionState(data, sessions, 'Connected');
        var dirty = hasSessionState(data, sessions, 'Dirty');
        currentSystemState = runtime.state || 'Idle';
        var effectiveState = connected ? 'Isolated Connected' : (dirty ? 'Isolated Dirty' : currentSystemState);

        stateEl.textContent = effectiveState + ' on ' + (data.os || 'unknown OS');
        stateEl.className = 'status-pill ' + (effectiveState.indexOf('Connected') !== -1 ? 'ok' : (effectiveState.indexOf('Dirty') !== -1 ? 'warn' : ''));
        connectButton.textContent = currentSystemState === 'Connected' || currentSystemState === 'Dirty' ? 'Disconnect' : 'Connect';

        endpointEl.value = gui.endpoint_ip || endpointEl.value || '';
        wstunnelEl.value = gui.wstunnel_binary || wstunnelEl.value || '';
        isolatedSessionEl.value = gui.isolated_session || isolatedSessionEl.value || '';
        if (!isolatedSessionEl.value && sessions.length === 1) isolatedSessionEl.value = sessions[0].session_id || '';
        isolatedCommandEl.value = gui.isolated_command || isolatedCommandEl.value || '';
        if (gui.last_output && (!lastRenderedOutput || lastRenderedOutput === 'Loading status...')) setOutput(gui.last_output);

        if (data.profile) {
          profileSummaryEl.innerHTML = definitionList([
            ['server', text(data.profile.server) + ':' + text(data.profile.port)],
            ['gateway', data.profile.wstunnel_url],
            ['addresses', join(data.profile.addresses)],
            ['allowed IPs', join(data.profile.allowed_ips)],
            ['fingerprint', data.profile.fingerprint],
            ['saved profile', gui.profile_path]
          ]);
        } else {
          profileSummaryEl.innerHTML = '<span class="warn">' + escapeHTML(data.error || 'no profile saved') + '</span>';
        }

        runtimeEl.innerHTML = definitionList([
          ['app version', data.version && data.version.version ? data.version.version : ''],
          ['cli path', data.cli_path || ''],
          ['privilege helper', data.privilege_helper || ''],
          ['profile path', gui.profile_path || ''],
          ['last command', gui.last_command || ''],
          ['last command time', gui.last_command_time || ''],
          ['state', runtime.state || ''],
          ['runtime root', runtime.runtime_root || ''],
          ['profile fingerprint', runtime.active ? runtime.active.profile_fingerprint : ''],
          ['interface', runtime.active ? runtime.active.wireguard_interface : ''],
          ['dns interface', runtime.active && runtime.active.dns_applied ? runtime.active.dns_interface : ''],
          ['dns servers', runtime.active && runtime.active.dns_applied ? join(runtime.active.dns_servers) : ''],
          ['isolated state', data.isolated ? data.isolated.state : ''],
          ['isolated root', data.isolated ? data.isolated.runtime_root : ''],
          ['isolated session', isolated ? isolated.session_id : ''],
          ['isolated namespace', isolated ? isolated.namespace : ''],
          ['isolated app pid', isolated && isolated.app_process ? isolated.app_process.pid : ''],
          ['isolated gateway pid', isolated && isolated.wstunnel_process ? isolated.wstunnel_process.pid : ''],
          ['isolated monitor pid', isolated && isolated.monitor_process ? isolated.monitor_process.pid : ''],
          ['isolated error', data.isolated ? data.isolated.error || '' : ''],
          ['known isolated sessions', knownSessionsText(sessions)],
          ['error', runtime.error || data.error || '']
        ]);
      }

      function refresh() {
        return requestJSON('GET', '/api/status').then(function (result) {
          if (!result.ok) throw new Error(failedResponseMessage(result, 'status request failed'));
          render(result.data);
          return result.data;
        });
      }

      function actionPayload() {
        return JSON.stringify({
          endpoint_ip: endpointEl.value,
          wstunnel_binary: wstunnelEl.value,
          session_id: isolatedSessionEl.value,
          app_command: isolatedCommandEl.value
        });
      }

      function runTask(label, task) {
        setBusy(true, label + '...');
        task().then(function (message) {
          return refresh().then(function () {
            if (message) setOutput(message);
          }, function (error) {
            setOutput(label + ' completed, but status refresh failed: ' + describeError(error));
          });
        }, function (error) {
          return refresh().then(function () {
            setOutput(label + ' failed: ' + describeError(error));
          }, function () {
            setOutput(label + ' failed: ' + describeError(error));
          });
        }).then(function () {
          setBusy(false);
        }, function (error) {
          setBusy(false);
          setOutput(label + ' failed: ' + describeError(error));
        });
      }

      function runAction(label, path) {
        runTask(label, function () {
          return requestJSON('POST', path, actionPayload(), { 'Content-Type': 'application/json' }).then(function (result) {
            if (!result.ok) throw new Error(failedResponseMessage(result, label + ' failed'));
            return responseMessage(result, label + ' completed');
          });
        });
      }

      function uploadProfile() {
        if (!window.FormData) {
          setOutput('this browser does not support profile upload');
          return;
        }
        var file = profileFileEl.files && profileFileEl.files.length ? profileFileEl.files[0] : null;
        if (!file) {
          setOutput('select a profile file first');
          return;
        }
        runTask('Saving profile', function () {
          var form = new FormData();
          form.append('profile', file);
          return requestJSON('POST', '/api/profile', form).then(function (result) {
            if (!result.ok) throw new Error(failedResponseMessage(result, 'profile upload failed'));
            var data = result.data || {};
            var gui = data.gui || {};
            return gui.last_output || responseMessage(result, 'profile saved');
          });
        });
      }

      function systemTogglePath() {
        return currentSystemState === 'Connected' || currentSystemState === 'Dirty' ? '/api/disconnect' : '/api/connect';
      }

      window.addEventListener('error', function (event) {
        setOutput('GUI JavaScript error: ' + describeError(event.error || event.message));
      });
      window.addEventListener('unhandledrejection', function (event) {
        setOutput('GUI request error: ' + describeError(event.reason));
      });

      document.getElementById('profile-form').addEventListener('submit', function (event) {
        event.preventDefault();
        uploadProfile();
      });
      profileFileEl.addEventListener('change', uploadProfile);
      connectButton.addEventListener('click', function () { runAction(connectButton.textContent || 'Connect', systemTogglePath()); });
      disconnectButton.addEventListener('click', function () { runAction('Disconnect', '/api/disconnect'); });
      preflightButton.addEventListener('click', function () { runAction('Preflight', '/api/preflight'); });
      diagnosticsButton.addEventListener('click', function () { runAction('Diagnostics', '/api/diagnostics'); });
      diagnosticsBundleButton.addEventListener('click', function () { runAction('Diagnostics bundle', '/api/diagnostics-bundle'); });
      isolatedPreflightButton.addEventListener('click', function () { runAction('Isolated preflight', '/api/isolated/preflight'); });
      isolatedStartButton.addEventListener('click', function () { runAction('Start isolated app', '/api/isolated/start'); });
      isolatedStopButton.addEventListener('click', function () { runAction('Stop isolated app', '/api/isolated/stop'); });
      isolatedCleanupButton.addEventListener('click', function () { runAction('Cleanup isolated app', '/api/isolated/cleanup'); });
      isolatedRecoverButton.addEventListener('click', function () { runAction('Recover isolated sessions', '/api/isolated/recover'); });
      refreshButton.addEventListener('click', function () {
        runTask('Refreshing status', function () {
          return refresh().then(function () { return 'status refreshed'; });
        });
      });

      setOutput('Loading status...');
      refresh().then(function () {
        if (lastRenderedOutput === 'Loading status...') setOutput('GUI ready.');
      }, function (error) {
        setOutput('Status refresh failed: ' + describeError(error));
      });
    }());
  </script>
</body>
</html>
`
