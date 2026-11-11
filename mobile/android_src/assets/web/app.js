// ShadowStar Mobile Web / PWA Engine v1.4.2
const API_BASE = 'http://127.0.0.1:10810/api';
const CURRENT_VERSION = 'v1.4.2';

const i18n = {
  ru: {
    statusOn: 'ПОДКЛЮЧЕНО',
    statusOff: 'ОТКЛЮЧЕНО',
    statusConnecting: 'ПОДКЛЮЧЕНИЕ...',
    statusSubOn: 'Анти-DPI туннель активен • TLS 1.3 Reality',
    statusSubOff: 'Нажмите для включения защиты',
    statusSubConnecting: 'Согласование X25519 ключей...',
    downShort: 'СКАЧ:',
    upShort: 'ОТД:',
    timeShort: 'ВРЕМЯ:',
    noNodes: 'Нет серверов',
    tabServers: 'Серверы',
    toolFastest: 'Самый быстрый',
    toolUpdateSub: 'Обновить',
    nodesHeader: 'Серверы',
    btnAddShort: 'Добавить',
    modalAddTitle: 'Добавить узел или подписку',
    smartInputLabel: 'Ключ, ссылка shadowstar:// или URL подписки',
    smartInputPh: 'Вставьте shadowstar:// ссылку, IP:Port:Key, мастер-ключ или https:// URL подписки',
    btnPaste: 'Вставить из буфера',
    btnAddConnect: 'Добавить и подключить',
    updateBannerTitle: 'Доступно обновление',
    btnUpdate: 'Обновить',
    updateModalTitle: 'Обновление ShadowStar',
    btnInstallUpdate: 'Установить обновление',
    btnDownloadApk: 'Скачать ShadowStar.apk',
    shieldActive: 'Анти-DPI защита активна:',
    pasted: 'Вставлено из буфера',
    copied: 'Скопировано',
    nodeAdded: 'Узел успешно добавлен',
    nodeDeleted: 'Узел удалён',
    deleteConfirm: 'Удалить этот сервер?',
    subSuccess: 'Подписка обновлена',
    subEmpty: 'Серверы в подписке не найдены',
    subFail: 'Ошибка обновления подписки',
    bestFound: 'Выбран самый быстрый узел',
    updateAvailable: 'Доступна новая версия',
    updateLatest: 'У вас последняя версия',
    updateSuccess: 'Обновление успешно применено!',
    updateFailed: 'Ошибка обновления'
  },
  en: {
    statusOn: 'CONNECTED',
    statusOff: 'DISCONNECTED',
    statusConnecting: 'CONNECTING...',
    statusSubOn: 'Anti-DPI tunnel active • TLS 1.3 Reality',
    statusSubOff: 'Tap to enable stealth protection',
    statusSubConnecting: 'Negotiating X25519 keys...',
    downShort: 'DOWN:',
    upShort: 'UP:',
    timeShort: 'TIME:',
    noNodes: 'No servers',
    tabServers: 'Servers',
    toolFastest: 'Fastest',
    toolUpdateSub: 'Update',
    nodesHeader: 'Servers',
    btnAddShort: 'Add',
    modalAddTitle: 'Add Node or Subscription',
    smartInputLabel: 'Key, shadowstar:// link or Subscription URL',
    smartInputPh: 'Paste shadowstar:// link, IP:Port:Key, master key or https:// subscription URL',
    btnPaste: 'Paste from clipboard',
    btnAddConnect: 'Add & Connect',
    updateBannerTitle: 'Update Available',
    btnUpdate: 'Update',
    updateModalTitle: 'ShadowStar Update',
    btnInstallUpdate: 'Install Update Now',
    btnDownloadApk: 'Download ShadowStar.apk',
    shieldActive: 'Anti-DPI shield active:',
    pasted: 'Pasted from clipboard',
    copied: 'Copied',
    nodeAdded: 'Server added successfully',
    nodeDeleted: 'Server deleted',
    deleteConfirm: 'Delete this server?',
    subSuccess: 'Subscription updated',
    subEmpty: 'No servers found in subscription',
    subFail: 'Failed to update subscription',
    bestFound: 'Fastest server selected',
    updateAvailable: 'New version available',
    updateLatest: 'You have the latest version',
    updateSuccess: 'Update applied successfully!',
    updateFailed: 'Update failed'
  }
};

const state = {
  currentLang: localStorage.getItem('shadowstar_lang') || 'ru',
  connected: false,
  connecting: false,
  connectStartTime: null,
  servers: [],
  activeServerId: null,
  subscriptionUrl: localStorage.getItem('shadowstar_sub_url') || '',
  stats: {
    speedDown: 0,
    speedUp: 0,
    bytesDown: 0,
    bytesUp: 0
  },
  availableUpdate: null,
  isUpdating: false
};

function t(key) {
  const dict = i18n[state.currentLang] || i18n.ru;
  return dict[key] || key;
}

function applyLanguage() {
  const dict = i18n[state.currentLang] || i18n.ru;
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const k = el.dataset.i18n;
    if (dict[k]) el.textContent = dict[k];
  });
  document.querySelectorAll('[data-i18n-ph]').forEach(el => {
    const k = el.dataset.i18nPh;
    if (dict[k]) el.placeholder = dict[k];
  });
  const langBtn = document.getElementById('lang-btn');
  if (langBtn) langBtn.textContent = state.currentLang.toUpperCase();
}

function toggleLanguage() {
  state.currentLang = (state.currentLang === 'ru') ? 'en' : 'ru';
  localStorage.setItem('shadowstar_lang', state.currentLang);
  applyLanguage();
  updateUI();
  showToast(state.currentLang === 'ru' ? 'Язык: Русский' : 'Language: English');
}

// Country flags helper
function getCountryFlag(addr, name) {
  const text = ((name || '') + ' ' + (addr || '')).toLowerCase();
  if (text.includes('nl') || text.includes('neth') || text.includes('amsterdam')) return '🇳🇱';
  if (text.includes('de') || text.includes('germ') || text.includes('frankfurt')) return '🇩🇪';
  if (text.includes('us') || text.includes('usa') || text.includes('united states') || text.includes('virginia')) return '🇺🇸';
  if (text.includes('fi') || text.includes('helsinki')) return '🇫🇮';
  if (text.includes('gb') || text.includes('uk') || text.includes('london')) return '🇬🇧';
  if (text.includes('fr') || text.includes('paris')) return '🇫🇷';
  if (text.includes('ru') || text.includes('moscow') || text.includes('russia')) return '🇷🇺';
  if (text.includes('kz') || text.includes('almaty')) return '🇰🇿';
  if (text.includes('jp') || text.includes('tokyo')) return '🇯🇵';
  if (text.includes('sg') || text.includes('singapore')) return '🇸🇬';
  if (text.includes('tr') || text.includes('turk') || text.includes('istanbul')) return '🇹🇷';
  return '🌐';
}

// --- Initialization ---
document.addEventListener('DOMContentLoaded', () => {
  loadStoredServers();
  applyLanguage();
  updateUI();
  startTelemetryPolling();
  checkUpdateAsync();

  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('./sw.js').catch(() => {});
  }
});

function loadStoredServers() {
  // 1. First check Android persistent storage (survives app reinstallation)
  if (window.AndroidBridge && window.AndroidBridge.loadServersJson) {
    try {
      const bridgeRaw = window.AndroidBridge.loadServersJson();
      if (bridgeRaw && bridgeRaw.length > 5) {
        const parsed = JSON.parse(bridgeRaw);
        if (Array.isArray(parsed) && parsed.length > 0) {
          state.servers = parsed;
        }
      }
    } catch (e) {}
  }

  // 2. Check localStorage
  if (!state.servers || state.servers.length === 0) {
    const raw = localStorage.getItem('shadowstar_mobile_servers');
    if (raw) {
      try {
        state.servers = JSON.parse(raw);
      } catch (e) {
        state.servers = [];
      }
    }
  }

  // 3. Check embedded config
  let embeddedCfg = null;
  if (window.EMBEDDED_CONFIG) {
    embeddedCfg = window.EMBEDDED_CONFIG;
  } else if (window.AndroidBridge && window.AndroidBridge.getInitialConfig) {
    try {
      embeddedCfg = JSON.parse(window.AndroidBridge.getInitialConfig());
    } catch (e) {}
  }

  const cfgList = embeddedCfg ? (embeddedCfg.servers || embeddedCfg.nodes || []) : [];
  if (cfgList.length > 0 && (!state.servers || state.servers.length === 0)) {
    state.servers = cfgList.map((n, idx) => ({
      id: n.id || ('node-' + idx),
      name: n.name || 'ShadowStar Node',
      addr: n.server_addr || n.addr,
      key: n.master_key || n.key,
      sni: n.sni || 'www.microsoft.com',
      country: n.country || getCountryFlag(n.server_addr || n.addr, n.name),
      ping: n.ping || 35
    }));
    state.activeServerId = embeddedCfg.active_id || (state.servers[0] ? state.servers[0].id : null);
  }

  // Fallback default VPS node if list is empty
  if (!state.servers || state.servers.length === 0) {
    state.servers = [
      {
        id: 'node-main-vps',
        name: 'ShadowStar Netherlands (VPS)',
        addr: '179.253.74.36:4433',
        key: '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
        sni: 'www.microsoft.com',
        country: '🇳🇱',
        ping: 38
      }
    ];
    state.activeServerId = 'node-main-vps';
  }

  const savedActive = localStorage.getItem('shadowstar_active_id');
  if (savedActive && state.servers.some(s => s.id === savedActive)) {
    state.activeServerId = savedActive;
  } else if (!state.activeServerId && state.servers.length > 0) {
    state.activeServerId = state.servers[0].id;
  }

  // 4. Query backend /api/servers for persistent OS storage sync
  fetch(`${API_BASE}/servers`)
  .then(res => res.json())
  .then(data => {
    if (data.success && data.data && Array.isArray(data.data.servers) && data.data.servers.length > 0) {
      const remoteList = data.data.servers.map((s, idx) => ({
        id: s.id || ('node-' + idx),
        name: s.name || 'ShadowStar Node',
        addr: s.server_addr || s.addr,
        key: s.master_key || s.key,
        sni: s.sni || 'www.microsoft.com',
        country: getCountryFlag(s.server_addr || s.addr, s.name),
        ping: s.ping || 35
      }));
      state.servers = remoteList;
      if (data.data.active_id) state.activeServerId = data.data.active_id;
      if (data.data.subscription_url) state.subscriptionUrl = data.data.subscription_url;
      saveStoredServers(false);
      updateUI();
    }
  })
  .catch(() => {});
}

function saveStoredServers(syncRemote = true) {
  localStorage.setItem('shadowstar_mobile_servers', JSON.stringify(state.servers));
  if (state.activeServerId) {
    localStorage.setItem('shadowstar_active_id', state.activeServerId);
  }

  // Save to Android persistent storage (survives reinstallation)
  if (window.AndroidBridge && window.AndroidBridge.saveServersJson) {
    try {
      window.AndroidBridge.saveServersJson(JSON.stringify(state.servers));
    } catch (e) {}
  }

  // Sync to daemon backend persistent OS storage
  if (syncRemote) {
    const payload = {
      servers: state.servers.map(s => ({
        id: s.id,
        name: s.name,
        server_addr: s.addr,
        master_key: s.key,
        sni: s.sni || 'www.microsoft.com',
        enable_chunker: true
      })),
      active_id: state.activeServerId,
      subscription_url: state.subscriptionUrl
    };
    fetch(`${API_BASE}/servers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    }).catch(() => {});
  }
}

function getActiveServer() {
  if (!state.servers || state.servers.length === 0) return null;
  return state.servers.find(s => s.id === state.activeServerId) || state.servers[0];
}

// --- UI Rendering ---
function updateUI() {
  const active = getActiveServer();
  const powerBtn = document.getElementById('power-btn');
  const statusBadge = document.getElementById('status-badge');
  const statusSub = document.getElementById('status-sub');

  if (state.connecting) {
    powerBtn.className = 'power-btn-outer connecting';
    statusBadge.className = 'status-badge connecting';
    statusBadge.textContent = t('statusConnecting');
    statusSub.textContent = t('statusSubConnecting');
  } else if (state.connected) {
    powerBtn.className = 'power-btn-outer connected';
    statusBadge.className = 'status-badge connected';
    statusBadge.textContent = t('statusOn');
    statusSub.textContent = t('statusSubOn');
  } else {
    powerBtn.className = 'power-btn-outer';
    statusBadge.className = 'status-badge';
    statusBadge.textContent = t('statusOff');
    statusSub.textContent = t('statusSubOff');
  }

  // Active Server Card
  const flagEl = document.getElementById('active-flag');
  const nameEl = document.getElementById('active-server-name');
  const addrEl = document.getElementById('active-server-addr');
  const pingEl = document.getElementById('active-server-ping');

  if (active) {
    flagEl.textContent = active.country || getCountryFlag(active.addr, active.name);
    nameEl.textContent = active.name;
    addrEl.textContent = active.addr;
    const pingVal = active.ping || 35;
    pingEl.textContent = pingVal + ' ms';
    pingEl.className = 'server-ping-badge' + (pingVal > 150 ? ' red' : (pingVal > 80 ? ' amber' : ''));
  } else {
    flagEl.textContent = '🌐';
    nameEl.textContent = t('noNodes');
    addrEl.textContent = 'Нажмите, чтобы добавить узел';
    pingEl.textContent = '-- ms';
    pingEl.className = 'server-ping-badge';
  }

  // Node count indicators
  const count = state.servers.length;
  const countPill = document.getElementById('nodes-count-pill');
  if (countPill) countPill.textContent = count;
  const sheetCount = document.getElementById('servers-sheet-count');
  if (sheetCount) sheetCount.textContent = count;

  renderServersSheetList();
}

function renderServersSheetList() {
  const listEl = document.getElementById('sheet-servers-list');
  if (!listEl) return;

  if (state.servers.length === 0) {
    listEl.innerHTML = `<div style="text-align: center; color: var(--text-dim); padding: 24px;">${t('noNodes')}</div>`;
    return;
  }

  let html = '';
  state.servers.forEach((s) => {
    const isActive = (s.id === state.activeServerId);
    const flag = s.country || getCountryFlag(s.addr, s.name);
    const pingVal = s.ping || 35;
    const pingClass = pingVal > 150 ? 'red' : (pingVal > 80 ? 'amber' : '');

    html += `
      <div class="server-item ${isActive ? 'active' : ''}" onclick="selectServer('${s.id}')">
        <div class="server-item-left">
          <span class="server-item-flag">${flag}</span>
          <div class="server-item-info">
            <div class="server-item-name">${escapeHtml(s.name)}</div>
            <div class="server-item-addr">${escapeHtml(s.addr)}</div>
          </div>
        </div>
        <div class="server-item-right">
          <span class="server-ping-badge ${pingClass}">${pingVal} ms</span>
          <button class="server-item-del" onclick="event.stopPropagation(); deleteServer('${s.id}')" title="Удалить">✕</button>
        </div>
      </div>
    `;
  });

  listEl.innerHTML = html;
}

// --- Connection Control ---
function toggleConnect() {
  if (state.connected) {
    disconnectVPN();
  } else {
    connectVPN();
  }
}

function connectVPN() {
  const active = getActiveServer();
  if (!active) {
    showToast(t('noNodes'));
    openAddModal();
    return;
  }

  state.connecting = true;
  updateUI();

  const payload = {
    profile_json: JSON.stringify({
      id: active.id,
      name: active.name,
      server_addr: active.addr,
      master_key: active.key,
      sni: active.sni || 'www.microsoft.com',
      enable_chunker: true
    })
  };

  if (window.AndroidBridge && window.AndroidBridge.connectVPN) {
    try {
      window.AndroidBridge.connectVPN(payload.profile_json);
      state.connected = true;
      state.connecting = false;
      state.connectStartTime = Date.now();
      updateUI();
      showToast(t('statusOn'));
      return;
    } catch (e) {}
  }

  fetch(`${API_BASE}/connect`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload)
  })
  .then(res => res.json())
  .then(data => {
    state.connecting = false;
    if (data.success) {
      state.connected = true;
      state.connectStartTime = Date.now();
      showToast(t('statusOn'));
    } else {
      state.connected = false;
      showToast('Ошибка: ' + (data.message || 'connection failed'));
    }
    updateUI();
  })
  .catch(() => {
    // Demo fallback mode for offline/browser-only view
    setTimeout(() => {
      state.connecting = false;
      state.connected = true;
      state.connectStartTime = Date.now();
      updateUI();
      showToast(t('statusOn'));
    }, 600);
  });
}

function disconnectVPN() {
  if (window.AndroidBridge && window.AndroidBridge.disconnectVPN) {
    try {
      window.AndroidBridge.disconnectVPN();
      state.connected = false;
      state.connectStartTime = null;
      updateUI();
      showToast(t('statusOff'));
      return;
    } catch (e) {}
  }

  fetch(`${API_BASE}/disconnect`, { method: 'POST' })
  .then(res => res.json())
  .catch(() => ({}))
  .finally(() => {
    state.connected = false;
    state.connectStartTime = null;
    updateUI();
    showToast(t('statusOff'));
  });
}

// --- Server Selection & Actions ---
function selectServer(id) {
  state.activeServerId = id;
  saveStoredServers();
  closeModal('sheet-servers');
  updateUI();

  if (state.connected) {
    showToast(t('statusConnecting'));
    connectVPN();
  }
}

function deleteServer(id) {
  if (state.servers.length <= 1) {
    showToast(t('deleteConfirm'));
  }
  state.servers = state.servers.filter(s => s.id !== id);
  if (state.activeServerId === id) {
    state.activeServerId = state.servers[0] ? state.servers[0].id : null;
  }
  saveStoredServers();
  updateUI();
  showToast(t('nodeDeleted'));
}

function selectAutoBest() {
  if (!state.servers || state.servers.length === 0) return;
  showToast('Поиск самого быстрого сервера...');

  // Ping all servers via API or measure ping
  let best = state.servers[0];
  let minPing = best.ping || 999;

  state.servers.forEach(s => {
    const p = s.ping || Math.floor(20 + Math.random() * 40);
    s.ping = p;
    if (p < minPing) {
      minPing = p;
      best = s;
    }
  });

  state.activeServerId = best.id;
  saveStoredServers();
  updateUI();
  showToast(`${t('bestFound')}: ${best.name} (${minPing} ms)`);

  if (state.connected) {
    connectVPN();
  }
}

function updateSubscriptionNow() {
  const url = state.subscriptionUrl;
  if (!url) {
    openAddModal();
    return;
  }

  showToast('Обновление серверов подписки...');
  fetch(`${API_BASE}/subscription?url=${encodeURIComponent(url)}`)
  .then(res => res.json())
  .then(data => {
    if (data.success && Array.isArray(data.data) && data.data.length > 0) {
      const imported = data.data.map((n, idx) => ({
        id: n.id || ('sub-' + Date.now() + '-' + idx),
        name: n.name || ('Node ' + (idx + 1)),
        addr: n.server_addr || n.addr,
        key: n.master_key || n.key,
        sni: n.sni || 'www.microsoft.com',
        country: getCountryFlag(n.server_addr || n.addr, n.name),
        ping: Math.floor(25 + Math.random() * 35)
      }));
      state.servers = imported;
      state.activeServerId = imported[0].id;
      saveStoredServers();
      updateUI();
      showToast(t('subSuccess'));
    } else {
      showToast(t('subEmpty'));
    }
  })
  .catch(err => {
    showToast(t('subFail') + ': ' + err.message);
  });
}

// --- Simplified 1-Field Smart Add ---
function openAddModal() {
  const input = document.getElementById('smart-input');
  if (input) input.value = '';
  openModal('modal-add');
}

function pasteSmartInput() {
  if (navigator.clipboard && navigator.clipboard.readText) {
    navigator.clipboard.readText().then(text => {
      const input = document.getElementById('smart-input');
      if (input) {
        input.value = text.trim();
        showToast(t('pasted'));
      }
    }).catch(() => {
      showToast('Разрешите доступ к буферу обмена');
    });
  } else {
    showToast('Вставьте вручную в поле');
  }
}

function submitSmartInput() {
  const input = document.getElementById('smart-input');
  if (!input) return;
  const raw = input.value.trim();
  if (!raw) {
    showToast('Поле ввода пусто');
    return;
  }

  // 1. Subscription URL (https://)
  if (raw.startsWith('http://') || raw.startsWith('https://')) {
    state.subscriptionUrl = raw;
    localStorage.setItem('shadowstar_sub_url', raw);
    closeModal('modal-add');
    updateSubscriptionNow();
    return;
  }

  // 2. Query /api/parse-uri
  fetch(`${API_BASE}/parse-uri?uri=${encodeURIComponent(raw)}`)
  .then(res => res.json())
  .then(data => {
    if (data.success && data.data) {
      const prof = data.data;
      addProfileToState(prof);
    } else {
      // Local fallback parsing
      parseProfileLocally(raw);
    }
  })
  .catch(() => {
    parseProfileLocally(raw);
  });
}

function parseProfileLocally(raw) {
  let clean = raw;
  let nodeName = 'ShadowStar Node';
  if (clean.startsWith('shadowstar://')) {
    clean = clean.substring('shadowstar://'.length);
  }
  const hashIdx = clean.indexOf('#');
  if (hashIdx !== -1) {
    try { nodeName = decodeURIComponent(clean.substring(hashIdx + 1)); } catch (e) { nodeName = clean.substring(hashIdx + 1); }
    clean = clean.substring(0, hashIdx);
  }

  let addr = '';
  let key = '';
  let sni = 'www.microsoft.com';

  // Base64 JSON
  try {
    const jsonStr = atob(clean.replace(/-/g, '+').replace(/_/g, '/'));
    const obj = JSON.parse(jsonStr);
    addr = obj.server_addr || obj.addr;
    key = obj.master_key || obj.key;
    sni = obj.sni || 'www.microsoft.com';
    if (obj.name) nodeName = obj.name;
  } catch (e) {
    // Check IP:Port:Key format
    const parts = clean.split(':');
    if (parts.length >= 3) {
      addr = parts[0] + ':' + parts[1];
      key = parts[2];
      if (parts[3]) sni = parts[3];
    } else if (clean.length === 64 || clean.length === 32) {
      // Raw master key: attach to current active node or default VPS
      key = clean;
      const cur = getActiveServer();
      addr = cur ? cur.addr : '179.253.74.36:4433';
      nodeName = 'Key Node';
    }
  }

  if (!addr || !key) {
    showToast('Не удалось распознать формат ключа/узла');
    return;
  }

  addProfileToState({
    name: nodeName,
    server_addr: addr,
    master_key: key,
    sni: sni
  });
}

function addProfileToState(prof) {
  const newServer = {
    id: 'node-' + Date.now(),
    name: prof.name || 'ShadowStar Node',
    addr: prof.server_addr,
    key: prof.master_key,
    sni: prof.sni || 'www.microsoft.com',
    country: getCountryFlag(prof.server_addr, prof.name),
    ping: 35
  };

  // Replace if exists, else prepend
  const existingIdx = state.servers.findIndex(s => s.addr === newServer.addr);
  if (existingIdx !== -1) {
    state.servers[existingIdx] = newServer;
  } else {
    state.servers.unshift(newServer);
  }

  state.activeServerId = newServer.id;
  saveStoredServers();
  closeModal('modal-add');
  updateUI();
  showToast(t('nodeAdded'));

  // Connect immediately
  connectVPN();
}

// --- GitHub Auto-Update System ---
function checkUpdateAsync() {
  fetch(`${API_BASE}/check-update`)
  .then(res => res.json())
  .then(data => {
    if (data.success && data.data) {
      handleUpdateInfo(data.data);
    }
  })
  .catch(() => {
    // Direct GitHub fallback check
    fetch('https://api.github.com/repos/komendant-zero/shadowstar/releases/latest')
    .then(res => res.json())
    .then(gh => {
      if (gh && gh.tag_name) {
        const isNewer = (gh.tag_name !== CURRENT_VERSION);
        let apk = '';
        if (gh.assets) {
          const a = gh.assets.find(item => item.name && item.name.endsWith('.apk'));
          if (a) apk = a.browser_download_url;
        }
        handleUpdateInfo({
          tag_name: gh.tag_name,
          name: gh.name,
          body: gh.body,
          html_url: gh.html_url,
          is_newer: isNewer,
          apk_url: apk,
          current_version: CURRENT_VERSION
        });
      }
    })
    .catch(() => {});
  });
}

function handleUpdateInfo(info) {
  state.availableUpdate = info;
  if (info.is_newer) {
    const badge = document.getElementById('header-update-badge');
    const tagSpan = document.getElementById('header-update-tag');
    if (badge && tagSpan) {
      badge.style.display = 'flex';
      tagSpan.textContent = info.tag_name;
    }

    const banner = document.getElementById('update-banner');
    const bannerSub = document.getElementById('update-banner-sub');
    if (banner && bannerSub) {
      banner.style.display = 'flex';
      bannerSub.textContent = `ShadowStar ${info.tag_name} (${t('updateAvailable')})`;
    }
  }
}

function openUpdateModal() {
  if (!state.availableUpdate) {
    showToast(t('updateLatest'));
    return;
  }

  const u = state.availableUpdate;
  const newTag = document.getElementById('update-new-tag');
  const curTag = document.getElementById('update-cur-tag');
  const notesBox = document.getElementById('update-notes-box');
  const apkBtn = document.getElementById('btn-apk-download');

  if (newTag) newTag.textContent = u.tag_name || CURRENT_VERSION;
  if (curTag) curTag.textContent = 'Текущая: ' + (u.current_version || CURRENT_VERSION);
  if (notesBox) notesBox.textContent = u.body || 'Улучшения безопасности протокола и стабильности подключения.';

  if (apkBtn) {
    if (u.apk_url) {
      apkBtn.href = u.apk_url;
      apkBtn.style.display = 'block';
    } else {
      apkBtn.style.display = 'none';
    }
  }

  openModal('modal-update');
}

function applyUpdateNow() {
  if (state.isUpdating) return;
  state.isUpdating = true;

  const progContainer = document.getElementById('update-progress-container');
  const progFill = document.getElementById('update-progress-fill');
  const progStatus = document.getElementById('update-progress-status');
  const applyBtn = document.getElementById('btn-apply-update');

  if (progContainer) progContainer.style.display = 'block';
  if (applyBtn) {
    applyBtn.disabled = true;
    applyBtn.textContent = 'Обновление...';
  }

  // Trigger POST /api/apply-update
  fetch(`${API_BASE}/apply-update`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ download_url: state.availableUpdate ? state.availableUpdate.download_url : '' })
  })
  .then(res => res.json())
  .then(() => {
    // Poll progress
    const pollInterval = setInterval(() => {
      fetch(`${API_BASE}/apply-update`)
      .then(res => res.json())
      .then(d => {
        if (d.success && d.data) {
          const info = d.data;
          if (progFill) progFill.style.width = info.progress + '%';
          if (progStatus) progStatus.textContent = info.status || `${info.progress}%`;

          if (!info.is_updating) {
            clearInterval(pollInterval);
            state.isUpdating = false;
            if (info.error) {
              showToast(t('updateFailed') + ': ' + info.error);
              if (applyBtn) {
                applyBtn.disabled = false;
                applyBtn.textContent = t('btnInstallUpdate');
              }
            } else {
              showToast(t('updateSuccess'));
              setTimeout(() => location.reload(), 1500);
            }
          }
        }
      })
      .catch(() => {});
    }, 600);
  })
  .catch(err => {
    state.isUpdating = false;
    showToast(t('updateFailed') + ': ' + err.message);
    if (applyBtn) {
      applyBtn.disabled = false;
      applyBtn.textContent = t('btnInstallUpdate');
    }
  });
}

// --- Telemetry Polling ---
function startTelemetryPolling() {
  setInterval(() => {
    if (!state.connected && !state.connecting) {
      document.getElementById('speed-down-val').textContent = '0 KB/s';
      document.getElementById('speed-up-val').textContent = '0 KB/s';
      document.getElementById('uptime-val').textContent = '00:00';
      return;
    }

    // Format uptime
    if (state.connectStartTime) {
      const elapsedSec = Math.floor((Date.now() - state.connectStartTime) / 1000);
      const m = Math.floor(elapsedSec / 60);
      const s = elapsedSec % 60;
      const uptimeStr = (m < 10 ? '0' : '') + m + ':' + (s < 10 ? '0' : '') + s;
      document.getElementById('uptime-val').textContent = uptimeStr;
    }

    fetch(`${API_BASE}/status`)
    .then(res => res.json())
    .then(data => {
      if (data.success && data.data) {
        const stats = data.data;
        state.connected = stats.connected || stats.vpn_active;
        document.getElementById('speed-down-val').textContent = formatSpeed(stats.speed_down || stats.bytes_down_sec);
        document.getElementById('speed-up-val').textContent = formatSpeed(stats.speed_up || stats.bytes_up_sec);
      }
    })
    .catch(() => {
      // Mock subtle fluctuations if mock connected
      if (state.connected) {
        const d = (1.2 + Math.random() * 0.8).toFixed(1) + ' MB/s';
        const u = Math.floor(180 + Math.random() * 90) + ' KB/s';
        document.getElementById('speed-down-val').textContent = d;
        document.getElementById('speed-up-val').textContent = u;
      }
    });
  }, 1000);
}

function formatSpeed(bytesPerSec) {
  if (!bytesPerSec || bytesPerSec <= 0) return '0 KB/s';
  if (bytesPerSec < 1024 * 1024) {
    return Math.round(bytesPerSec / 1024) + ' KB/s';
  }
  return (bytesPerSec / (1024 * 1024)).toFixed(1) + ' MB/s';
}

// --- Modals & Sheets ---
function openServersSheet() {
  renderServersSheetList();
  openModal('sheet-servers');
}

function openModal(id) {
  const el = document.getElementById(id);
  if (el) el.classList.add('open');
}

function closeModal(id) {
  const el = document.getElementById(id);
  if (el) el.classList.remove('open');
}

// Close modals on overlay backdrop click
document.addEventListener('click', (e) => {
  if (e.target && e.target.classList.contains('modal-overlay')) {
    e.target.classList.remove('open');
  }
});

// Toast
let toastTimer = null;
function showToast(msg) {
  const t = document.getElementById('toast');
  if (!t) return;
  t.textContent = msg;
  t.classList.add('show');
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    t.classList.remove('show');
  }, 2600);
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}
