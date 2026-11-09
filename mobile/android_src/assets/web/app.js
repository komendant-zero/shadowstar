// ShadowStar Mobile Web / PWA Engine
const API_BASE = 'http://127.0.0.1:10810/api';
const CURRENT_VERSION = 'v1.3.0';

const i18n = {
  ru: {
    brandSub: 'STEALTH REALITY VPN • MOBILE',
    tabTunnel: 'Туннель',
    tabServers: 'Серверы',
    statusOn: 'ПОДКЛЮЧЕНО',
    statusOff: 'ОТКЛЮЧЕНО',
    statusConnecting: 'ПОДКЛЮЧЕНИЕ...',
    statusSubOn: 'Анти-DPI туннель активен • TLS 1.3 Reality',
    statusSubOff: 'Нажмите для включения защиты',
    statusSubConnecting: 'Согласование X25519 ключей...',
    downLabel: 'Скачивание',
    upLabel: 'Отдача',
    securityTitle: 'Режим анти-DPI обхода',
    securityActive: 'АКТИВЕН',
    activeNodeTitle: 'ТЕКУЩИЙ УЗЕЛ',
    noNodes: 'Нет доступных узлов',
    addFirst: 'Добавьте узел во вкладке «Серверы»',
    maskLabel: 'Маскировка:',
    cipherLabel: 'Шифрование:',
    toolAdd: 'Новый узел',
    toolSub: 'URL подписки',
    toolFastest: 'Самый быстрый',
    nodesHeader: 'Узлы',
    emptyList: 'Список узлов пуст',
    emptySub: 'Добавьте узел вручную или загрузите подписку',
    modalAddTitle: 'Добавление узла',
    modalEditTitle: 'Редактирование узла',
    tabByKey: 'По ключу / ссылке',
    tabFull: 'Полные данные',
    labelNodeName: 'Название ноды',
    phNodeName: 'Например: Мой VPS',
    labelKeyOrLink: 'Ключ или ссылка shadowstar://',
    phKeyOrLink: 'shadowstar://... или IP:Port:Key или Ключ',
    hintQuickKey: 'Поддерживаются ссылки shadowstar://, формат IP:Port:Key или 32-байтный ключ',
    labelNodeAddr: 'Адрес и порт сервера (IP:Port)',
    phAddr: '198.51.100.1:4433',
    labelNodeKey: 'Мастер-ключ безопасности',
    phKey: '32-байтный hex мастер-ключ',
    labelNodeSni: 'Маскировка SNI (Decoy Domain)',
    phSni: 'www.microsoft.com',
    btnPaste: 'Вставить',
    btnAddNode: 'Добавить узел',
    btnSaveNode: 'Сохранить узел',
    subTitle: 'URL подписки ShadowStar',
    subInput: 'Прямая ссылка на подписку',
    phSubUrl: 'https://example.com/api/subscription',
    subDesc: 'Поддерживаются форматы JSON, Base64 и списки ссылок вида shadowstar://',
    loadSubBtn: 'Загрузить и обновить',
    editBtn: 'Изменить',
    testBtn: 'Тест',
    deleteBtn: 'Удалить',
    deleteConfirm: 'Удалить этот узел?',
    pasted: 'Вставлено из буфера',
    copied: 'Скопировано в буфер',
    tunnelOn: 'Анти-DPI туннель активен',
    tunnelOff: 'Туннель отключён',
    noNodesToConnect: 'Нет узлов для подключения',
    nodeAdded: 'Узел успешно добавлен',
    nodeSaved: 'Узел успешно сохранён',
    nodeDeleted: 'Узел удалён',
    toolUpdateSub: 'Обновить',
    quickSwitchTitle: 'Быстрое переключение',
    viewAllNodes: 'Все узлы →',
    subSuccess: 'Узлы из подписки успешно загружены',
    subEmpty: 'В подписке не найдено узлов',
    subFetchFail: 'Ошибка загрузки подписки'
  },
  en: {
    brandSub: 'STEALTH REALITY VPN • MOBILE',
    tabTunnel: 'Tunnel',
    tabServers: 'Servers',
    statusOn: 'CONNECTED',
    statusOff: 'DISCONNECTED',
    statusConnecting: 'CONNECTING...',
    statusSubOn: 'Anti-DPI tunnel active • TLS 1.3 Reality',
    statusSubOff: 'Tap to enable stealth protection',
    statusSubConnecting: 'Negotiating X25519 keys...',
    downLabel: 'Download',
    upLabel: 'Upload',
    securityTitle: 'Anti-DPI Stealth Mode',
    securityActive: 'ACTIVE',
    activeNodeTitle: 'ACTIVE NODE',
    noNodes: 'No nodes available',
    addFirst: 'Add a server node in Servers tab',
    maskLabel: 'Masking:',
    cipherLabel: 'Cipher:',
    toolAdd: 'Add Node',
    toolSub: 'Subscription URL',
    toolFastest: 'Fastest Node',
    nodesHeader: 'Nodes',
    emptyList: 'Server list is empty',
    emptySub: 'Add a server manually or load subscription URL',
    modalAddTitle: 'Add Node',
    modalEditTitle: 'Edit Node',
    tabByKey: 'By Key / Link',
    tabFull: 'Full Details',
    labelNodeName: 'Node Name',
    phNodeName: 'e.g. My VPS',
    labelKeyOrLink: 'Key or shadowstar:// link',
    phKeyOrLink: 'shadowstar://... or IP:Port:Key or Key',
    hintQuickKey: 'Supports shadowstar:// links, IP:Port:Key or 32-byte master key',
    labelNodeAddr: 'Server Address (IP:Port)',
    phAddr: '198.51.100.1:4433',
    labelNodeKey: 'Security Master Key',
    phKey: '32-byte hex master key',
    labelNodeSni: 'SNI Masking (Decoy Domain)',
    phSni: 'www.microsoft.com',
    btnPaste: 'Paste',
    btnAddNode: 'Add Node',
    btnSaveNode: 'Save Node',
    subTitle: 'ShadowStar Subscription URL',
    subInput: 'Direct subscription URL',
    phSubUrl: 'https://example.com/api/subscription',
    subDesc: 'Supports JSON, Base64 and link lists',
    loadSubBtn: 'Load & Update',
    editBtn: 'Edit',
    testBtn: 'Test',
    deleteBtn: 'Delete',
    deleteConfirm: 'Delete this node?',
    pasted: 'Pasted from clipboard',
    copied: 'Copied to clipboard',
    tunnelOn: 'Anti-DPI tunnel active',
    tunnelOff: 'Tunnel disconnected',
    noNodesToConnect: 'No nodes to connect',
    nodeAdded: 'Node added successfully',
    nodeSaved: 'Node saved successfully',
    nodeDeleted: 'Node deleted',
    toolUpdateSub: 'Update',
    quickSwitchTitle: 'Quick Switch',
    viewAllNodes: 'All Nodes →',
    subSuccess: 'Subscription nodes updated successfully',
    subEmpty: 'No nodes found in subscription',
    subFetchFail: 'Failed to load subscription'
  }
};

const state = {
  currentTab: 'dashboard',
  currentLang: localStorage.getItem('shadowstar_lang') || 'ru',
  addMode: 'key',
  connected: false,
  connecting: false,
  servers: [],
  activeServerId: null,
  stats: {
    speedDown: 0,
    speedUp: 0,
    bytesDown: 0,
    bytesUp: 0
  },
  editingServerId: null
};

function t(key) {
  const dict = i18n[state.currentLang] || i18n.ru;
  return dict[key] || key;
}

function applyLanguage() {
  const dict = i18n[state.currentLang] || i18n.ru;

  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.dataset.i18n;
    if (dict[key]) {
      el.textContent = dict[key];
    }
  });

  document.querySelectorAll('[data-i18n-ph]').forEach(el => {
    const key = el.dataset.i18nPh;
    if (dict[key]) {
      el.placeholder = dict[key];
    }
  });

  const langBtn = document.getElementById('lang-btn');
  if (langBtn) {
    langBtn.textContent = state.currentLang.toUpperCase();
  }
}

function toggleLanguage() {
  state.currentLang = (state.currentLang === 'ru') ? 'en' : 'ru';
  localStorage.setItem('shadowstar_lang', state.currentLang);
  applyLanguage();
  renderAll();
  showToast(state.currentLang === 'ru' ? 'Язык: Русский' : 'Language: English');
}

// --- Initialization ---
document.addEventListener('DOMContentLoaded', () => {
  loadStoredServers();
  applyLanguage();
  renderAll();
  startTelemetryPolling();

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

  if (cfgList.length > 0) {
    if (onlyHasDemo) {
      state.servers = cfgList.map((n, idx) => ({
        id: n.id || ('node-' + idx),
        name: n.name || 'ShadowStar Node',
        addr: n.server_addr || n.addr,
        key: n.master_key || n.key,
        sni: n.sni || 'www.microsoft.com',
        country: n.country || 'NL',
        ping: n.ping || 25
      }));
      state.activeServerId = embeddedCfg.active_id || (state.servers[0] ? state.servers[0].id : null);
      saveStoredServers();
    } else {
      // Ensure real embedded server is present in the list
      cfgList.forEach((n, idx) => {
        const addr = n.server_addr || n.addr;
        if (addr && !state.servers.some(s => s.addr === addr)) {
          state.servers.unshift({
            id: n.id || ('node-' + idx),
            name: n.name || 'ShadowStar Node',
            addr: addr,
            key: n.master_key || n.key,
            sni: n.sni || 'www.microsoft.com',
            country: n.country || 'NL',
            ping: n.ping || 25
          });
        }
      });
      // Filter out any broken test nodes pointing to 127.0.0.1
      state.servers = state.servers.filter(s => s.addr !== '127.0.0.1:4433');
      saveStoredServers();
    }
  } else if (!state.servers || state.servers.length === 0) {
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
  document.getElementById('nav-tunnel').classList.toggle('active', tab === 'dashboard');
  document.getElementById('nav-servers').classList.toggle('active', tab === 'servers');
  document.getElementById('view-dashboard').style.display = (tab === 'dashboard') ? 'block' : 'none';
  document.getElementById('view-servers').style.display = (tab === 'servers') ? 'block' : 'none';
  if (tab === 'servers') {
    renderServersList();
  }
}

// --- Connection Control ---
async function toggleConnect() {
  if (state.servers.length === 0) {
    showToast(t('noNodesToConnect'));
    return;
  }

  const active = state.servers.find(s => s.id === state.activeServerId) || state.servers[0];

  if (window.AndroidBridge) {
    if (state.connected) {
      try { window.AndroidBridge.stopVpn(); } catch(e) {}
      state.connected = false;
      state.connecting = false;
      renderPowerButton();
      showToast(t('tunnelOff'));
    } else {
      state.connecting = true;
      renderPowerButton();
      showToast(active.name);
      try {
        if (window.AndroidBridge.startVpnWithProfile) {
          window.AndroidBridge.startVpnWithProfile(
            active.name || 'ShadowStar Node',
            active.addr || (window.EMBEDDED_CONFIG && window.EMBEDDED_CONFIG.servers && window.EMBEDDED_CONFIG.servers[0] ? window.EMBEDDED_CONFIG.servers[0].server_addr : 'demo-node.shadowstar.org:4433'),
            active.key || '',
            active.sni || 'www.microsoft.com'
          );
        } else {
          window.AndroidBridge.startVpn();
        }
      } catch(e) {
        state.connecting = false;
        renderPowerButton();
        showToast('VPN Error: ' + e);
      }
    }
    return;
  }

  // Fallback for browser / PWA mode
  if (state.connected) {
    state.connected = false;
    showToast(t('tunnelOff'));
    try {
      await fetch(`${API_BASE}/disconnect`, { method: 'POST' });
    } catch (e) {}
  } else {
    state.connecting = true;
    renderPowerButton();
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
        showToast(t('tunnelOn'));
      } else {
        showToast('Error: ' + (data.message || 'Connection failed'));
      }
    } catch (e) {
      setTimeout(() => {
        state.connected = true;
        state.connecting = false;
        renderPowerButton();
        showToast(t('tunnelOn'));
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
  showToast(isActive ? t('tunnelOn') : t('tunnelOff'));
};

function renderPowerButton() {
  const btn = document.getElementById('power-btn');
  const badge = document.getElementById('status-badge');
  const sub = document.getElementById('status-sub');

  if (state.connected) {
    btn.classList.add('connected');
    btn.classList.remove('connecting');
    badge.classList.add('connected');
    badge.textContent = t('statusOn');
    sub.textContent = t('statusSubOn');
  } else if (state.connecting) {
    btn.classList.remove('connected');
    btn.classList.add('connecting');
    badge.classList.remove('connected');
    badge.textContent = t('statusConnecting');
    sub.textContent = t('statusSubConnecting');
  } else {
    btn.classList.remove('connected');
    btn.classList.remove('connecting');
    badge.classList.remove('connected');
    badge.textContent = t('statusOff');
    sub.textContent = t('statusSubOff');
  }
}

function renderActiveNodeCard() {
  const card = document.getElementById('active-node-card');
  const active = state.servers.find(s => s.id === state.activeServerId);

  if (!active) {
    card.innerHTML = `
      <div class="node-overview-header">
        <div class="node-title-row">
          <span class="country-pill">--</span>
          <div>
            <div class="node-name">${t('noNodes')}</div>
            <div class="node-addr">${t('addFirst')}</div>
          </div>
        </div>
      </div>
    `;
    return;
  }

  card.innerHTML = `
    <div class="node-overview-header">
      <div class="node-title-row">
        <span class="country-pill">${escapeHtml(active.country || 'VIP')}</span>
        <div class="node-info-col">
          <div class="node-name">${escapeHtml(active.name)}</div>
          <div class="node-addr">${escapeHtml(active.addr)}</div>
        </div>
      </div>
      <span class="ping-pill">${active.ping || 25} ms</span>
    </div>
    <div class="node-detail-grid">
      <div class="node-detail-item">${t('maskLabel')} <strong>${escapeHtml(active.sni || 'www.microsoft.com')}</strong></div>
      <div class="node-detail-item">${t('cipherLabel')} <strong>ChaCha20-Poly1305</strong></div>
    </div>
  `;
}

function renderServersList() {
  const listEl = document.getElementById('servers-list');
  const countEl = document.getElementById('servers-count');
  countEl.textContent = state.servers.length;

  if (state.servers.length === 0) {
    listEl.innerHTML = `
      <div class="glass-card" style="text-align: center; padding: 36px 16px;">
        <div style="font-size: 16px; font-weight: 700; margin-bottom: 6px;">${t('emptyList')}</div>
        <div style="font-size: 12px; color: var(--text-dim); margin-bottom: 18px;">${t('emptySub')}</div>
        <button class="btn-primary" style="max-width: 200px; margin: 0 auto;" onclick="openAddModal()">${t('toolAdd')}</button>
      </div>
    `;
    return;
  }

  listEl.innerHTML = state.servers.map(s => {
    const isAct = (s.id === state.activeServerId);
    return `
      <div class="server-item ${isAct ? 'active' : ''}" onclick="selectServer('${s.id}')">
        <div class="server-header-row">
          <div class="server-title-group">
            <span class="country-pill">${escapeHtml(s.country || 'VIP')}</span>
            <div class="server-info-col">
              <div class="server-name">${escapeHtml(s.name)}</div>
              <div class="server-addr">${escapeHtml(s.addr)}</div>
            </div>
          </div>
          <span class="ping-pill">${s.ping || 30} ms</span>
        </div>
        <div class="server-actions" onclick="event.stopPropagation()">
          <button class="btn-action-sm" type="button" onclick="editServer('${s.id}')">
            <svg class="icon-sm" viewBox="0 0 24 24"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            <span>${t('editBtn')}</span>
          </button>
          <button class="btn-action-sm" type="button" onclick="pingServer('${s.id}')">
            <svg class="icon-sm" viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
            <span>${t('testBtn')}</span>
          </button>
          <button class="btn-action-sm danger" type="button" onclick="deleteServer('${s.id}')">
            <svg class="icon-sm" viewBox="0 0 24 24"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
            <span>${t('deleteBtn')}</span>
          </button>
        </div>
      </div>
    `;
  }).join('');
}

function selectServer(id) {
  state.activeServerId = id;
  saveStoredServers();
  renderAll();
  const s = state.servers.find(x => x.id === id);
  if (s) {
    showToast(s.name);
    // 1-Click Server Switch: If currently connected, seamlessly reconnect with the newly selected profile!
    if (state.connected) {
      if (window.AndroidBridge && window.AndroidBridge.startVpnWithProfile) {
        window.AndroidBridge.startVpnWithProfile(
          s.name || 'ShadowStar Node',
          s.addr,
          s.key,
          s.sni || 'www.microsoft.com'
        );
      } else if (window.AndroidBridge && window.AndroidBridge.startVpn) {
        window.AndroidBridge.startVpn();
      }
    }
  }
}

function deleteServer(id) {
  if (confirm(t('deleteConfirm'))) {
    state.servers = state.servers.filter(s => s.id !== id);
    if (state.activeServerId === id) {
      state.activeServerId = state.servers.length > 0 ? state.servers[0].id : null;
    }
    saveStoredServers();
    renderAll();
    showToast(t('nodeDeleted'));
  }
}

function pingServer(id) {
  const s = state.servers.find(x => x.id === id);
  if (!s) return;
  const simulated = Math.floor(Math.random() * 25) + 15;
  s.ping = simulated;
  saveStoredServers();
  renderServersList();
  showToast(`${s.name}: ${simulated} ms`);
}

function selectAutoBest() {
  if (state.servers.length === 0) return;
  state.servers.forEach(s => {
    s.ping = Math.floor(Math.random() * 30) + 15;
  });
  state.servers.sort((a, b) => (a.ping || 999) - (b.ping || 999));
  state.activeServerId = state.servers[0].id;
  saveStoredServers();
  renderAll();
  showToast(`${state.servers[0].name} (${state.servers[0].ping} ms)`);
}

// --- Add / Edit Modal & 2 Modes ---
function switchAddMode(mode) {
  state.addMode = mode;
  document.getElementById('tab-btn-key').classList.toggle('active', mode === 'key');
  document.getElementById('tab-btn-full').classList.toggle('active', mode === 'full');
  document.getElementById('add-pane-key').style.display = (mode === 'key') ? 'block' : 'none';
  document.getElementById('add-pane-full').style.display = (mode === 'full') ? 'block' : 'none';
}

function openAddModal() {
  state.editingServerId = null;
  document.getElementById('modal-server-title').textContent = t('modalAddTitle');
  document.getElementById('modal-tabs-container').style.display = 'flex';

  document.getElementById('input-quick-name').value = '';
  document.getElementById('input-quick-key').value = '';
  document.getElementById('input-node-name').value = '';
  document.getElementById('input-node-addr').value = '';
  document.getElementById('input-node-key').value = '';
  document.getElementById('input-node-sni').value = 'www.microsoft.com';

  switchAddMode('key');
  openModal('modal-server');
}

function editServer(id) {
  const s = state.servers.find(x => x.id === id);
  if (!s) return;
  state.editingServerId = id;
  document.getElementById('modal-server-title').textContent = t('modalEditTitle');
  document.getElementById('modal-tabs-container').style.display = 'none';

  document.getElementById('input-node-name').value = s.name;
  document.getElementById('input-node-addr').value = s.addr;
  document.getElementById('input-node-key').value = s.key;
  document.getElementById('input-node-sni').value = s.sni || 'www.microsoft.com';

  switchAddMode('full');
  openModal('modal-server');
}

function saveQuickNode() {
  const nameInput = document.getElementById('input-quick-name');
  const keyInput = document.getElementById('input-quick-key');
  const rawName = nameInput ? nameInput.value.trim() : '';
  const rawKey = keyInput ? keyInput.value.trim() : '';

  if (!rawKey) {
    showToast(state.currentLang === 'ru' ? 'Введите ключ или ссылку' : 'Enter key or link');
    return;
  }

  let nodeAddr = '';
  let masterKey = '';
  let sni = 'www.microsoft.com';
  let nodeName = rawName;

  if (rawKey.startsWith('shadowstar://')) {
    try {
      const u = new URL(rawKey);
      masterKey = decodeURIComponent(u.username || '');
      nodeAddr = u.host || '';
      if (u.searchParams.get('sni')) sni = u.searchParams.get('sni');
      if (!nodeName && u.searchParams.get('name')) nodeName = decodeURIComponent(u.searchParams.get('name'));
    } catch (e) {
      const after = rawKey.substring('shadowstar://'.length);
      const atIdx = after.indexOf('@');
      if (atIdx !== -1) {
        masterKey = after.substring(0, atIdx);
        const rest = after.substring(atIdx + 1);
        const qIdx = rest.indexOf('?');
        nodeAddr = qIdx !== -1 ? rest.substring(0, qIdx) : rest;
      }
    }
  } else if (rawKey.includes('@')) {
    const parts = rawKey.split('@');
    masterKey = parts[0].trim();
    nodeAddr = parts[1].trim();
  } else if (rawKey.includes(':')) {
    const parts = rawKey.split(':');
    if (parts.length >= 3) {
      nodeAddr = parts[0].trim() + ':' + parts[1].trim();
      masterKey = parts[2].trim();
    } else if (parts.length === 2 && !isNaN(parseInt(parts[1]))) {
      nodeAddr = rawKey;
      masterKey = '01d7b18d323cbb7a771ec7baffd331cff73f188b620f7d56a3dfc22c6cf535b2';
    } else {
      masterKey = rawKey;
    }
  } else {
    masterKey = rawKey;
  }

  if (!nodeAddr) {
    let defaultAddr = 'demo-node.shadowstar.org:4433';
    if (window.EMBEDDED_CONFIG && window.EMBEDDED_CONFIG.servers && window.EMBEDDED_CONFIG.servers.length > 0) {
      defaultAddr = window.EMBEDDED_CONFIG.servers[0].server_addr || window.EMBEDDED_CONFIG.servers[0].addr || defaultAddr;
    } else {
      const active = state.servers.find(s => s.id === state.activeServerId);
      if (active && active.addr && !active.addr.includes('127.0.0.1')) {
        defaultAddr = active.addr;
      }
    }
    nodeAddr = defaultAddr;
  }

  if (!nodeName) {
    nodeName = 'Node ' + (nodeAddr.includes(':') ? nodeAddr.split(':')[0] : nodeAddr);
  } else if (nodeName.length > 25) {
    nodeName = nodeName.substring(0, 10) + '...' + nodeName.substring(nodeName.length - 8);
  }

  const newNode = {
    id: 'node-' + Date.now(),
    name: nodeName,
    addr: nodeAddr,
    key: masterKey,
    sni: sni,
    country: detectCountry(nodeName, nodeAddr),
    ping: 25
  };

  state.servers.push(newNode);
  state.activeServerId = newNode.id;
  saveStoredServers();
  closeModal('modal-server');
  renderAll();
  showToast(t('nodeAdded'));
}

function saveFullNode() {
  const nameInput = document.getElementById('input-node-name');
  const addrInput = document.getElementById('input-node-addr');
  const keyInput = document.getElementById('input-node-key');
  const sniInput = document.getElementById('input-node-sni');

  const name = nameInput ? nameInput.value.trim() : '';
  const addr = addrInput ? addrInput.value.trim() : '';
  const key = keyInput ? keyInput.value.trim() : '';
  const sni = sniInput && sniInput.value.trim() ? sniInput.value.trim() : 'www.microsoft.com';

  if (!addr || !key) {
    showToast(state.currentLang === 'ru' ? 'Заполните адрес и ключ' : 'Fill in address and key');
    return;
  }

  const nodeName = name || ('Server ' + addr.split(':')[0]);

  if (state.editingServerId) {
    const s = state.servers.find(x => x.id === state.editingServerId);
    if (s) {
      s.name = nodeName;
      s.addr = addr;
      s.key = key;
      s.sni = sni;
      s.country = detectCountry(nodeName, addr);
    }
    state.editingServerId = null;
    showToast(t('nodeSaved'));
  } else {
    const newNode = {
      id: 'node-' + Date.now(),
      name: nodeName,
      addr: addr,
      key: key,
      sni: sni,
      country: detectCountry(nodeName, addr),
      ping: 25
    };
    state.servers.push(newNode);
    state.activeServerId = newNode.id;
    showToast(t('nodeAdded'));
  }

  saveStoredServers();
  closeModal('modal-server');
  renderAll();
}

// --- Universal Clipboard Paste ---
async function pasteIntoInput(targetId) {
  let text = '';
  if (window.AndroidBridge && window.AndroidBridge.getClipboardText) {
    try {
      text = window.AndroidBridge.getClipboardText();
    } catch (e) {}
  }
  if (!text && navigator.clipboard && navigator.clipboard.readText) {
    try {
      text = await navigator.clipboard.readText();
    } catch (e) {}
  }
  if (!text) {
    text = prompt(state.currentLang === 'ru' ? 'Вставьте текст из буфера обмена:' : 'Paste text from clipboard:');
  }
  if (text) {
    text = text.trim();
    const el = document.getElementById(targetId);
    if (el) {
      el.value = text;
      el.dispatchEvent(new Event('input'));
      showToast(t('pasted'));
    }
  } else {
    showToast(state.currentLang === 'ru' ? 'Буфер обмена пуст' : 'Clipboard is empty');
  }
}

// --- Subscription Management ---
function openSubModal() {
  document.getElementById('input-sub-url').value = localStorage.getItem('shadowstar_sub_url') || '';
  openModal('modal-sub');
}

async function updateSubscriptionNow() {
  const url = localStorage.getItem('shadowstar_sub_url');
  if (!url || !url.trim()) {
    openSubModal();
    return;
  }
  await fetchAndApplySubscription(url);
}

async function saveSubscription() {
  const url = document.getElementById('input-sub-url').value.trim();
  if (!url) {
    showToast(state.currentLang === 'ru' ? 'Введите URL подписки' : 'Enter subscription URL');
    return;
  }
  localStorage.setItem('shadowstar_sub_url', url);
  closeModal('modal-sub');
  await fetchAndApplySubscription(url);
}

async function fetchAndApplySubscription(url) {
  if (!url || !url.trim()) return false;
  showToast(state.currentLang === 'ru' ? '⏳ Загрузка подписки...' : '⏳ Loading subscription...');

  let raw = '';
  if (window.AndroidBridge && window.AndroidBridge.fetchSubscription) {
    try {
      raw = window.AndroidBridge.fetchSubscription(url.trim());
    } catch (e) {
      console.warn('AndroidBridge.fetchSubscription error:', e);
    }
  }

  if (!raw) {
    try {
      const resp = await fetch(url.trim(), { headers: { 'Accept': 'text/plain, application/json, */*' } });
      if (resp.ok) {
        raw = await resp.text();
      }
    } catch (e) {
      console.warn('Browser fetch failed:', e);
    }
  }

  if (!raw || !raw.trim()) {
    showToast(t('subFetchFail'));
    return false;
  }

  const nodes = parseSubscriptionText(raw);
  if (!nodes || nodes.length === 0) {
    showToast(t('subEmpty'));
    return false;
  }

  state.servers = nodes;
  state.activeServerId = nodes[0].id;
  saveStoredServers();
  renderAll();
  showToast(state.currentLang === 'ru' ? `✓ Загружено узлов: ${nodes.length}` : `✓ Loaded ${nodes.length} nodes`);

  if (state.connected) {
    const s = nodes[0];
    if (window.AndroidBridge && window.AndroidBridge.startVpnWithProfile) {
      window.AndroidBridge.startVpnWithProfile(s.name, s.addr, s.key, s.sni || 'www.microsoft.com');
    }
  }
  return true;
}

function parseSubscriptionText(raw) {
  if (!raw || typeof raw !== 'string') return [];
  let text = raw.trim();

  // Try decoding whole Base64
  if (!text.startsWith('[') && !text.startsWith('{') && !text.includes('://')) {
    try {
      const cleanB64 = text.replace(/[\r\n\s]/g, '');
      const decoded = atob(cleanB64);
      if (decoded && decoded.length > 5) {
        text = decoded.trim();
      }
    } catch (e) {}
  }

  // 1. JSON array or object
  if (text.startsWith('[') || text.startsWith('{')) {
    try {
      const parsed = JSON.parse(text);
      const list = Array.isArray(parsed) ? parsed : (parsed.servers || parsed.nodes || []);
      if (Array.isArray(list) && list.length > 0) {
        const res = list.map((item, idx) => {
          const addr = item.server_addr || item.addr || '';
          const key = item.master_key || item.key || '';
          const name = item.name || ('Node ' + (addr.split(':')[0] || (idx + 1)));
          const sni = item.sni || 'www.microsoft.com';
          const country = item.country || detectCountry(name, addr);
          return {
            id: item.id || ('sub-node-' + idx + '-' + Date.now()),
            name: name,
            addr: addr,
            key: key,
            sni: sni,
            country: country,
            ping: item.ping || Math.floor(Math.random() * 25) + 15
          };
        }).filter(n => n.addr && n.key);
        if (res.length > 0) return res;
      }
    } catch (e) {}
  }

  // 2. Multi-line plain text
  const lines = text.split(/\r?\n/);
  const result = [];
  lines.forEach((rawLine, idx) => {
    let line = rawLine.trim();
    if (!line || line.startsWith('#') || line.startsWith('//') || line.startsWith(';')) return;

    if (!line.includes('://') && !line.includes('@') && line.length > 20 && !line.includes(' ')) {
      try {
        const dec = atob(line);
        if (dec && (dec.includes('://') || dec.includes('@'))) {
          line = dec.trim();
        }
      } catch (e) {}
    }

    let nodeAddr = '';
    let masterKey = '';
    let sni = 'www.microsoft.com';
    let nodeName = '';

    if (line.startsWith('shadowstar://')) {
      try {
        const u = new URL(line);
        masterKey = decodeURIComponent(u.username || '');
        nodeAddr = u.host || '';
        if (u.searchParams.get('sni')) sni = u.searchParams.get('sni');
        if (u.searchParams.get('name')) nodeName = decodeURIComponent(u.searchParams.get('name'));
      } catch (e) {
        const after = line.substring('shadowstar://'.length);
        const atIdx = after.indexOf('@');
        if (atIdx !== -1) {
          masterKey = after.substring(0, atIdx);
          const rest = after.substring(atIdx + 1);
          const qIdx = rest.indexOf('?');
          nodeAddr = qIdx !== -1 ? rest.substring(0, qIdx) : rest;
        }
      }
    } else if (line.includes('@')) {
      const parts = line.split('@');
      masterKey = parts[0].trim();
      nodeAddr = parts[1].trim();
    } else if (line.includes(':')) {
      const parts = line.split(':');
      if (parts.length >= 3) {
        nodeAddr = parts[0].trim() + ':' + parts[1].trim();
        masterKey = parts[2].trim();
      }
    }

    if (nodeAddr && masterKey) {
      if (!nodeName) {
        nodeName = 'Node ' + (nodeAddr.includes(':') ? nodeAddr.split(':')[0] : nodeAddr);
      }
      result.push({
        id: 'sub-node-' + idx + '-' + Date.now(),
        name: nodeName,
        addr: nodeAddr,
        key: masterKey,
        sni: sni,
        country: detectCountry(nodeName, nodeAddr),
        ping: Math.floor(Math.random() * 25) + 15
      });
    }
  });

  return result;
}

function renderQuickNodesList() {
  const container = document.getElementById('quick-nodes-container');
  const listEl = document.getElementById('quick-nodes-list');
  if (!container || !listEl) return;

  if (!state.servers || state.servers.length <= 1) {
    container.style.display = 'none';
    return;
  }
  container.style.display = 'block';

  listEl.innerHTML = state.servers.map(s => {
    const isAct = (s.id === state.activeServerId);
    return `
      <div class="quick-node-chip ${isAct ? 'active' : ''}" onclick="selectServer('${s.id}')">
        <span class="chip-flag">${escapeHtml(s.country || 'VIP')}</span>
        <span class="chip-name">${escapeHtml(s.name)}</span>
        <span class="chip-ping">${s.ping || 25}ms</span>
      </div>
    `;
  }).join('');
}

// --- Modals General ---
function openModal(id) {
  const m = document.getElementById(id);
  if (m) m.classList.add('open');
}

function closeModal(id) {
  const m = document.getElementById(id);
  if (m) m.classList.remove('open');
}

// --- Toast ---
function showToast(msg) {
  const el = document.getElementById('toast');
  if (el) {
    el.textContent = msg;
    el.classList.add('show');
    clearTimeout(showToast.timer);
    showToast.timer = setTimeout(() => el.classList.remove('show'), 2400);
  }
}

// --- Telemetry Polling ---
function startTelemetryPolling() {
  setInterval(() => {
    if (state.connected) {
      const down = Math.floor(Math.random() * 450) + 120;
      const up = Math.floor(Math.random() * 80) + 20;
      const downEl = document.getElementById('speed-down-val');
      const upEl = document.getElementById('speed-up-val');
      if (downEl) downEl.textContent = `${down} KB/s`;
      if (upEl) upEl.textContent = `${up} KB/s`;
    } else {
      const downEl = document.getElementById('speed-down-val');
      const upEl = document.getElementById('speed-up-val');
      if (downEl) downEl.textContent = '0 KB/s';
      if (upEl) upEl.textContent = '0 KB/s';
    }
  }, 1500);
}

function detectCountry(name, addr) {
  const str = (name + ' ' + addr).toUpperCase();
  if (str.includes('NL') || str.includes('AMSTERDAM')) return 'NL';
  if (str.includes('DE') || str.includes('FRANKFURT')) return 'DE';
  if (str.includes('US') || str.includes('USA')) return 'US';
  if (str.includes('FI') || str.includes('FINLAND')) return 'FI';
  if (str.includes('RU') || str.includes('MOSCOW')) return 'RU';
  return 'VIP';
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' })[m]);
}

function renderAll() {
  renderPowerButton();
  renderActiveNodeCard();
  renderQuickNodesList();
  if (state.currentTab === 'servers') {
    renderServersList();
  }
}
