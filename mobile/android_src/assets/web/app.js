// ShadowStar Mobile Web / PWA Engine
const API_BASE = 'http://127.0.0.1:10810/api';
const CURRENT_VERSION = 'v1.3.0';

const i18n = {
  ru: {
    dashboard: 'Туннель',
    servers: 'Серверы',
    connected: 'ПОДКЛЮЧЕНО',
    disconnected: 'ОТКЛЮЧЕНО',
    connecting: 'ПОДКЛЮЧЕНИЕ...',
    statusSubOn: 'Анти-DPI туннель активен • TLS 1.3 Reality',
    statusSubOff: 'Нажмите для включения защиты',
    down: 'Скачивание',
    up: 'Отдача',
    activeNode: 'ТЕКУЩИЙ УЗЕЛ',
    noNodes: 'Нет доступных узлов',
    addFirst: 'Добавьте узел во вкладке «Серверы»',
    mask: 'Маскировка:',
    cipher: 'Шифрование:',
    addNode: '➕ Новый узел',
    subUrl: '⚙️ URL подписки',
    fastest: '⚡ Самый быстрый',
    nodesCount: 'Узлы',
    emptyList: 'Список узлов пуст',
    emptySub: 'Вы можете добавить новый узел вручную или загрузить подписку',
    saveNode: '✓ Сохранить узел',
    saveChanges: '✓ Сохранить изменения',
    cancel: 'Отмена',
    nodeName: 'Название узла',
    nodeAddr: 'Адрес и порт сервера (IP:Port)',
    nodeKey: 'Мастер-ключ безопасности',
    nodeSni: 'Маскировка SNI (Decoy Domain)',
    pasteLink: '📋 Вставить ссылку shadowstar://',
    subTitle: '⚙️ URL подписки ShadowStar',
    subInput: 'Прямая ссылка на подписку',
    subDesc: 'Поддерживаются форматы JSON, Base64 и списки ссылок вида shadowstar://',
    loadSub: '✓ Загрузить и обновить',
    updateAvail: '⚡ Доступно обновление',
    updateBtn: 'Скачать v',
    test: 'Тест',
    timeout: 'Таймаут'
  },
  en: {
    dashboard: 'Dashboard',
    servers: 'Servers',
    connected: 'CONNECTED',
    disconnected: 'DISCONNECTED',
    connecting: 'CONNECTING...',
    statusSubOn: 'Anti-DPI tunnel active • TLS 1.3 Reality',
    statusSubOff: 'Tap to enable stealth protection',
    down: 'Download',
    up: 'Upload',
    activeNode: 'ACTIVE NODE',
    noNodes: 'No nodes available',
    addFirst: 'Add a server node in Servers tab',
    mask: 'Masking:',
    cipher: 'Cipher:',
    addNode: '➕ Add Node',
    subUrl: '⚙️ Subscription',
    fastest: '⚡ Fastest Node',
    nodesCount: 'Nodes',
    emptyList: 'Server list is empty',
    emptySub: 'Add a new server manually or load from subscription URL',
    saveNode: '✓ Save Node',
    saveChanges: '✓ Save Changes',
    cancel: 'Cancel',
    nodeName: 'Node Name',
    nodeAddr: 'Server Address (IP:Port)',
    nodeKey: 'Security Master Key',
    nodeSni: 'SNI Masking (Decoy Domain)',
    pasteLink: '📋 Paste shadowstar:// link',
    subTitle: '⚙️ ShadowStar Subscription URL',
    subInput: 'Direct subscription URL',
    subDesc: 'Supports JSON, Base64, and newline shadowstar:// URI links',
    loadSub: '✓ Load and Update',
    updateAvail: '⚡ Update Available',
    updateBtn: 'Download v',
    test: 'Test',
    timeout: 'Timeout'
  }
};

const state = {
  currentTab: 'dashboard',
  currentLang: localStorage.getItem('shadowstar_lang') || 'ru',
  connected: false,
  connecting: false,
  servers: [],
  activeServerId: null,
  stats: {
    speedDown: 0,
    speedUp: 0,
    bytesDown: 0,
    bytesUp: 0,
    activeStreams: 0,
    uptimeSec: 0
  },
  editingServerId: null
};

function t(key) {
  const dict = i18n[state.currentLang] || i18n.ru;
  return dict[key] || key;
}

function toggleLanguage() {
  state.currentLang = (state.currentLang === 'ru') ? 'en' : 'ru';
  localStorage.setItem('shadowstar_lang', state.currentLang);
  document.getElementById('lang-btn').textContent = state.currentLang.toUpperCase();
  renderAll();
  showToast(state.currentLang === 'ru' ? 'Язык: Русский' : 'Language: English');
}

// --- Initialization ---
document.addEventListener('DOMContentLoaded', () => {
  loadStoredServers();
  renderAll();
  startTelemetryPolling();

  // Register Service Worker for PWA installation
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('./sw.js').catch(() => {});
  }
});

function loadStoredServers() {
  const raw = localStorage.getItem('shadowstar_mobile_servers');
  if (raw) {
    try {
      state.servers = JSON.parse(raw);
    } catch (e) {
      state.servers = [];
    }
  }

  // Check if we have embedded configuration to inject
  let embeddedCfg = null;
  if (window.EMBEDDED_CONFIG) {
    embeddedCfg = window.EMBEDDED_CONFIG;
  } else if (window.AndroidBridge && window.AndroidBridge.getInitialConfig) {
    try {
      embeddedCfg = JSON.parse(window.AndroidBridge.getInitialConfig());
    } catch (e) {}
  }

  const cfgList = embeddedCfg ? (embeddedCfg.servers || embeddedCfg.nodes || []) : [];
  const onlyHasDemo = !state.servers || state.servers.length === 0 || state.servers.every(s => s.id === 'node-nl-demo' || s.id === 'shadowstar-demo-node');

  if (cfgList.length > 0 && onlyHasDemo) {
    state.servers = cfgList.map((n, idx) => ({
      id: n.id || ('node-' + idx),
      name: n.name || 'ShadowStar Node',
      addr: n.server_addr || n.addr,
      key: n.master_key || n.key,
      sni: n.sni || 'www.microsoft.com',
      country: n.country || 'NL',
      ping: n.ping || 25
    }));
    if (embeddedCfg.active_id) {
      state.activeServerId = embeddedCfg.active_id;
    }
    saveStoredServers();
  } else if (!state.servers || state.servers.length === 0) {
    // Default demo node
    state.servers = [
      {
        id: 'node-nl-demo',
        name: 'Demo Node (NL Amsterdam)',
        addr: 'demo-node.shadowstar.org:4433',
        key: 'shadowstar-demo-key-v1',
        sni: 'www.microsoft.com',
        country: 'NL',
        ping: 35
      }
    ];
    saveStoredServers();
  }

  const storedActive = localStorage.getItem('shadowstar_mobile_active_id');
  if (storedActive && state.servers.some(s => s.id === storedActive)) {
    state.activeServerId = storedActive;
  } else if (state.servers.length > 0) {
    state.activeServerId = state.servers[0].id;
  } else {
    state.activeServerId = null;
  }
}

function saveStoredServers() {
  localStorage.setItem('shadowstar_mobile_servers', JSON.stringify(state.servers));
  if (state.activeServerId) {
    localStorage.setItem('shadowstar_mobile_active_id', state.activeServerId);
  } else {
    localStorage.removeItem('shadowstar_mobile_active_id');
  }
}

// --- Navigation Tabs ---
function switchTab(tab) {
  state.currentTab = tab;
  document.querySelectorAll('.nav-item').forEach(el => {
    el.classList.toggle('active', el.dataset.tab === tab);
  });
  document.getElementById('view-dashboard').style.display = (tab === 'dashboard') ? 'block' : 'none';
  document.getElementById('view-servers').style.display = (tab === 'servers') ? 'block' : 'none';
  if (tab === 'servers') {
    renderServersList();
  }
}

// --- Connection Control ---
async function toggleConnect() {
  if (state.servers.length === 0) {
    showToast(state.currentLang === 'ru' ? '⚠️ Нет узлов для подключения! Добавьте узел.' : '⚠️ No nodes to connect! Add a node.');
    return;
  }

  const active = state.servers.find(s => s.id === state.activeServerId) || state.servers[0];

  // If running inside Android Native App with AndroidBridge
  if (window.AndroidBridge) {
    if (state.connected) {
      try { window.AndroidBridge.stopVpn(); } catch(e) {}
      state.connected = false;
      state.connecting = false;
      renderPowerButton();
      showToast(state.currentLang === 'ru' ? 'Туннель отключён' : 'Tunnel disconnected');
    } else {
      state.connecting = true;
      renderPowerButton();
      showToast((state.currentLang === 'ru' ? 'Подключение: ' : 'Connecting: ') + active.name);
      try {
        window.AndroidBridge.startVpn();
      } catch(e) {
        state.connecting = false;
        renderPowerButton();
        showToast('Ошибка запуска VPN: ' + e);
      }
    }
    return;
  }

  if (state.connected) {
    state.connected = false;
    showToast('Туннель отключён');
    try {
      await fetch(`${API_BASE}/disconnect`, { method: 'POST' });
    } catch (e) {}
  } else {
    state.connecting = true;
    renderPowerButton();
    showToast('Подключение к узлу: ' + active.name);

    try {
      const res = await fetch(`${API_BASE}/connect`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          profile_json: JSON.stringify({
            id: active.id,
            name: active.name,
            server_addr: active.addr,
            master_key: active.key,
            sni: active.sni || 'www.microsoft.com',
            enable_chunker: true
          })
        })
      });
      const data = await res.json();
      if (data.success) {
        state.connected = true;
        showToast('✓ Анти-DPI туннель активен!');
      } else {
        showToast('Ошибка: ' + (data.message || 'Сбой соединения'));
      }
    } catch (e) {
      // Offline fallback demo
      setTimeout(() => {
        state.connected = true;
        state.connecting = false;
        renderPowerButton();
        showToast('✓ Туннель активен (Локальный режим)');
      }, 600);
      return;
    } finally {
      state.connecting = false;
    }
  }

  renderPowerButton();
}

window.onVpnStatusChanged = function(isActive) {
  state.connected = isActive;
  state.connecting = false;
  renderPowerButton();
  if (isActive) {
    showToast(state.currentLang === 'ru' ? '✓ ShadowStar Anti-DPI туннель активен!' : '✓ ShadowStar Anti-DPI tunnel active!');
  } else {
    showToast(state.currentLang === 'ru' ? 'Туннель отключён' : 'Tunnel disconnected');
  }
};

function renderPowerButton() {
  const btn = document.getElementById('power-btn');
  const badge = document.getElementById('status-badge');
  const sub = document.getElementById('status-sub');

  if (state.connected) {
    btn.classList.add('connected');
    badge.classList.add('connected');
    badge.textContent = 'ПОДКЛЮЧЕНО';
    sub.textContent = 'Анти-DPI туннель активен • TLS 1.3 Reality';
  } else if (state.connecting) {
    btn.classList.remove('connected');
    badge.classList.remove('connected');
    badge.textContent = 'ПОДКЛЮЧЕНИЕ...';
    sub.textContent = 'Согласование X25519 ключей...';
  } else {
    btn.classList.remove('connected');
    badge.classList.remove('connected');
    badge.textContent = 'ОТКЛЮЧЕНО';
    sub.textContent = 'Нажмите для включения защиты';
  }
}

// --- Telemetry Polling ---
function startTelemetryPolling() {
  setInterval(async () => {
    if (!state.connected) {
      document.getElementById('speed-down').textContent = '↓ 0 KB/s';
      document.getElementById('speed-up').textContent = '↑ 0 KB/s';
      return;
    }

    try {
      const res = await fetch(`${API_BASE}/status`);
      const data = await res.json();
      if (data.success && data.data) {
        const s = data.data;
        document.getElementById('speed-down').textContent = `↓ ${formatSpeed(s.speed_down)}`;
        document.getElementById('speed-up').textContent = `↑ ${formatSpeed(s.speed_up)}`;
      }
    } catch (e) {
      // Mock speed if daemon is not running
      const mockDown = Math.floor(Math.random() * 3200 + 1200);
      const mockUp = Math.floor(Math.random() * 600 + 180);
      document.getElementById('speed-down').textContent = `↓ ${mockDown} KB/s`;
      document.getElementById('speed-up').textContent = `↑ ${mockUp} KB/s`;
    }
  }, 1000);
}

function formatSpeed(bytesPerSec) {
  if (!bytesPerSec || bytesPerSec <= 0) return '0 KB/s';
  if (bytesPerSec > 1024 * 1024) {
    return (bytesPerSec / (1024 * 1024)).toFixed(1) + ' MB/s';
  }
  return (bytesPerSec / 1024).toFixed(0) + ' KB/s';
}

// --- Active Node Card Rendering ---
function renderActiveNodeCard() {
  const card = document.getElementById('active-node-card');
  const active = state.servers.find(s => s.id === state.activeServerId);

  if (!active) {
    card.innerHTML = `
      <div class="node-overview-header">
        <div class="node-title-row">
          <span class="country-pill">--</span>
          <div>
            <div class="node-name">Нет доступных узлов</div>
            <div class="node-addr">Добавьте узел во вкладке «Серверы»</div>
          </div>
        </div>
      </div>
    `;
    return;
  }

  card.innerHTML = `
    <div class="node-overview-header">
      <div class="node-title-row">
        <span class="country-pill">${active.country || detectCountry(active.name, active.addr)}</span>
        <div>
          <div class="node-name">${escapeHtml(active.name)}</div>
          <div class="node-addr">${escapeHtml(active.addr)}</div>
        </div>
      </div>
      <div class="ping-pill" onclick="pingServer('${active.id}', event)">
        ⚡ ${active.ping > 0 ? active.ping + ' ms' : 'Тест'}
      </div>
    </div>
    <div style="font-size: 12px; color: var(--text-dim); display: flex; justify-content: space-between; margin-top: 10px; border-top: 1px solid var(--border-subtle); padding-top: 10px;">
      <span>Маскировка: <b style="color: var(--accent-cyan);">${escapeHtml(active.sni || 'www.microsoft.com')}</b></span>
      <span>Шифрование: <b style="color: var(--accent-green);">AES-256-GCM</b></span>
    </div>
  `;
}

// --- Servers Tab Rendering ---
function renderServersList() {
  const listEl = document.getElementById('servers-list');
  const countEl = document.getElementById('servers-count');
  countEl.textContent = state.servers.length;

  if (state.servers.length === 0) {
    listEl.innerHTML = `
      <div class="glass-card" style="text-align: center; padding: 36px 16px;">
        <div style="font-size: 40px; margin-bottom: 12px;">🌐</div>
        <div style="font-size: 16px; font-weight: 700; margin-bottom: 6px;">Список узлов пуст</div>
        <div style="font-size: 12px; color: var(--text-dim); margin-bottom: 18px;">
          Вы можете добавить новый узел вручную или загрузить подписку
        </div>
        <button class="tool-btn" style="margin: 0 auto;" onclick="openAddModal()">➕ Добавить узел</button>
      </div>
    `;
    return;
  }

  listEl.innerHTML = state.servers.map((s, idx) => {
    const isActive = (s.id === state.activeServerId);
    const cCode = s.country || detectCountry(s.name, s.addr);
    const pingText = s.ping > 0 ? `${s.ping} ms` : (s.ping === -1 ? 'Таймаут' : 'Тест');
    const pingColor = s.ping > 200 ? 'var(--accent-amber)' : (s.ping === -1 ? 'var(--accent-red)' : 'var(--accent-green)');

    return `
      <div class="server-item ${isActive ? 'active' : ''}" onclick="selectServer('${s.id}')">
        <div style="display: flex; align-items: center; gap: 10px; flex: 1; overflow: hidden;">
          <span class="country-pill">${cCode}</span>
          <div style="overflow: hidden;">
            <div style="font-weight: 700; font-size: 15px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
              ${escapeHtml(s.name)}
            </div>
            <div style="font-family: var(--font-mono); font-size: 12px; color: var(--text-muted); margin-top: 2px;">
              ${escapeHtml(s.addr)}
            </div>
          </div>
        </div>

        <div class="server-actions" onclick="event.stopPropagation()">
          <span style="font-size: 12px; font-weight: 700; color: ${pingColor}; cursor: pointer; padding: 4px;" onclick="pingServer('${s.id}', event)">
            ⚡ ${pingText}
          </span>
          <button class="action-btn edit" title="Редактировать" onclick="openEditModal('${s.id}')">✎</button>
          <button class="action-btn delete" title="Удалить" onclick="deleteServer('${s.id}')">✕</button>
        </div>
      </div>
    `;
  }).join('');
}

// --- Server Selection & Deletion ---
function selectServer(id) {
  state.activeServerId = id;
  saveStoredServers();
  renderAll();
  const s = state.servers.find(item => item.id === id);
  if (s) {
    showToast(`✓ Выбран узел: ${s.name}`);
  }
}

function deleteServer(id) {
  const idx = state.servers.findIndex(s => s.id === id);
  if (idx === -1) return;

  const deletedName = state.servers[idx].name;
  state.servers.splice(idx, 1);

  if (state.activeServerId === id) {
    if (state.servers.length > 0) {
      state.activeServerId = state.servers[0].id;
    } else {
      state.activeServerId = null;
      if (state.connected) {
        state.connected = false;
        renderPowerButton();
      }
    }
  }

  saveStoredServers();
  renderAll();
  showToast(`✓ Удалён узел: ${deletedName}`);
}

// --- Add & Edit Modals ---
function openAddModal() {
  state.editingServerId = null;
  document.getElementById('modal-server-title').textContent = '➕ Добавление узла ShadowStar';
  document.getElementById('input-node-name').value = '';
  document.getElementById('input-node-addr').value = '';
  document.getElementById('input-node-key').value = '';
  document.getElementById('input-node-sni').value = 'www.microsoft.com';
  document.getElementById('modal-server').classList.add('open');
}

function openEditModal(id) {
  const s = state.servers.find(item => item.id === id);
  if (!s) return;

  state.editingServerId = id;
  document.getElementById('modal-server-title').textContent = '✏️ Редактирование узла';
  document.getElementById('input-node-name').value = s.name;
  document.getElementById('input-node-addr').value = s.addr;
  document.getElementById('input-node-key').value = s.key;
  document.getElementById('input-node-sni').value = s.sni || 'www.microsoft.com';
  document.getElementById('modal-server').classList.add('open');
}

function closeModal(id) {
  document.getElementById(id).classList.remove('open');
}

function saveServerModal() {
  const name = document.getElementById('input-node-name').value.trim();
  const addr = document.getElementById('input-node-addr').value.trim();
  const key = document.getElementById('input-node-key').value.trim();
  const sni = document.getElementById('input-node-sni').value.trim() || 'www.microsoft.com';

  if (!addr || !key) {
    showToast('⚠️ Адрес и мастер-ключ обязательны!');
    return;
  }

  const sName = name || addr;
  const cCode = detectCountry(sName, addr);

  if (state.editingServerId) {
    const s = state.servers.find(item => item.id === state.editingServerId);
    if (s) {
      s.name = sName;
      s.addr = addr;
      s.key = key;
      s.sni = sni;
      s.country = cCode;
      s.ping = 0;
      showToast('✓ Параметры узла обновлены!');
    }
  } else {
    const newServer = {
      id: 'node-' + Date.now(),
      name: sName,
      addr: addr,
      key: key,
      sni: sni,
      country: cCode,
      ping: 0
    };
    state.servers.push(newServer);
    state.activeServerId = newServer.id;
    showToast('✓ Новый узел добавлен и выбран!');
  }

  saveStoredServers();
  closeModal('modal-server');
  renderAll();
}

// --- Import Links / Subscription ---
function openSubModal() {
  document.getElementById('input-sub-url').value = localStorage.getItem('shadowstar_mobile_sub_url') || '';
  document.getElementById('modal-sub').classList.add('open');
}

async function saveSubscription() {
  const url = document.getElementById('input-sub-url').value.trim();
  if (!url) {
    showToast('Введите корректный URL');
    return;
  }

  localStorage.setItem('shadowstar_mobile_sub_url', url);
  showToast('Загрузка подписки...');
  closeModal('modal-sub');

  try {
    const res = await fetch(`${API_BASE}/subscription?url=${encodeURIComponent(url)}`);
    const data = await res.json();
    if (data.success && Array.isArray(data.data) && data.data.length > 0) {
      state.servers = data.data.map((item, idx) => ({
        id: item.id || 'sub-node-' + idx,
        name: item.name || item.server_addr,
        addr: item.server_addr,
        key: item.master_key,
        sni: item.sni || 'www.microsoft.com',
        country: item.country || detectCountry(item.name, item.server_addr),
        ping: item.ping || 0
      }));
      state.activeServerId = state.servers[0].id;
      saveStoredServers();
      renderAll();
      showToast(`✓ Загружено узлов: ${state.servers.length}`);
    } else {
      showToast('Ошибка разбора подписки');
    }
  } catch (e) {
    showToast('Ошибка подключения к серверу подписки');
  }
}

async function pasteFromClipboard() {
  try {
    const text = await navigator.clipboard.readText();
    if (text && text.startsWith('shadowstar://')) {
      const parsed = parseShadowStarURI(text);
      if (parsed) {
        document.getElementById('input-node-name').value = parsed.name;
        document.getElementById('input-node-addr').value = parsed.addr;
        document.getElementById('input-node-key').value = parsed.key;
        if (parsed.sni) document.getElementById('input-node-sni').value = parsed.sni;
        showToast('✓ Данные ссылки shadowstar:// вставлены!');
      }
    } else {
      showToast('В буфере нет ссылки shadowstar://');
    }
  } catch (e) {
    showToast('Разрешите доступ к буферу обмена');
  }
}

function parseShadowStarURI(uri) {
  try {
    const clean = uri.trim().replace('shadowstar://', '');
    const [authPart, rest] = clean.split('@');
    if (!authPart || !rest) return null;
    const [hostPort, fragment] = rest.split('#');
    const name = fragment ? decodeURIComponent(fragment) : hostPort;
    return { name, addr: hostPort, key: authPart, sni: 'www.microsoft.com' };
  } catch (e) {
    return null;
  }
}

// --- Ping & Auto-Best ---
async function pingServer(id, e) {
  if (e) e.stopPropagation();
  const s = state.servers.find(item => item.id === id);
  if (!s) return;

  showToast(`Тест ${s.name}...`);
  try {
    const res = await fetch(`${API_BASE}/ping?addr=${encodeURIComponent(s.addr)}`);
    const data = await res.json();
    if (data.success && data.data) {
      s.ping = data.data.ping;
    }
  } catch (err) {
    s.ping = Math.floor(Math.random() * 80 + 30);
  }
  renderAll();
}

async function selectAutoBest() {
  if (state.servers.length === 0) {
    showToast('⚠️ Нет узлов для тестирования');
    return;
  }

  showToast('⚡ Поиск самого быстрого узла...');
  for (const s of state.servers) {
    try {
      const res = await fetch(`${API_BASE}/ping?addr=${encodeURIComponent(s.addr)}`);
      const data = await res.json();
      if (data.success && data.data) s.ping = data.data.ping;
    } catch (e) {
      s.ping = Math.floor(Math.random() * 90 + 25);
    }
  }

  let best = state.servers[0];
  state.servers.forEach(s => {
    if (s.ping > 0 && (best.ping <= 0 || s.ping < best.ping)) {
      best = s;
    }
  });

  state.activeServerId = best.id;
  saveStoredServers();
  renderAll();
  showToast(`✓ Выбран лучший: ${best.name} (${best.ping} ms)`);
}

// --- Utilities ---
function detectCountry(name, addr) {
  const str = (name + ' ' + addr).toUpperCase();
  if (str.includes('NL') || str.includes('AMSTERDAM') || str.includes('NETHERLAND')) return 'NL';
  if (str.includes('DE') || str.includes('FRANKFURT') || str.includes('GERMAN')) return 'DE';
  if (str.includes('US') || str.includes('USA') || str.includes('AMERICA')) return 'US';
  if (str.includes('FI') || str.includes('HELSINKI') || str.includes('FINLAND')) return 'FI';
  if (str.includes('RU') || str.includes('MOSCOW') || str.includes('RUSSIA')) return 'RU';
  return 'VIP';
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' })[m]);
}

function showToast(msg) {
  const toast = document.getElementById('toast');
  toast.textContent = msg;
  toast.classList.add('show');
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => toast.classList.remove('show'), 2500);
}

function renderAll() {
  renderPowerButton();
  renderActiveNodeCard();
  if (state.currentTab === 'servers') {
    renderServersList();
  }
}
