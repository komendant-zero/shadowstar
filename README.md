# ⭐ ShadowStar (SSP) • Next-Gen Anti-DPI Stealth Proxy

[![GitHub Release](https://img.shields.io/badge/release-v1.3.0-blue.svg)](https://github.com/shadowstar/shadowstar/releases)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20Android%20%7C%20iOS-green.svg)]()
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Anti-DPI](https://img.shields.io/badge/Anti--DPI-Reality%20%2B%20Chunker-emerald.svg)]()

> **[🇬🇧 English](#-english-overview) | [🇷🇺 Русский](#-обзор-на-русском)**

---

## 🇬🇧 English Overview

**ShadowStar (SSP)** is an advanced, high-performance stealth proxy protocol engineered from the ground up to bypass modern Deep Packet Inspection (**DPI, TSPU, GFW, SNI-filtering**) and resist active network probing.

It provides a complete cross-platform ecosystem:
- **Pure Win32 GDI+ Desktop Client** — A GPU-accelerated cyberpunk dark acrylic interface with zero Webview/Electron overhead (~15MB RAM).
- **Mobile Ecosystem (Android & iOS PWA)** — Built-in mobile gateway with offline Service Worker support, touch UX, and native Android `VpnService` integration.
- **Automated GitHub Updater** — In-app update checking with direct download and release notes.
- **Bilingual Interface** — Instant runtime switching between English and Russian (Desktop & Mobile).

```mermaid
flowchart LR
    subgraph ClientApp["💻 Desktop & 📱 Mobile Client"]
        direction TB
        UI["Modern UI (Win32 / Mobile Web)"]
        S5["SOCKS5 / HTTP Inbound\n:10808 / :10809"]
        Chunker["TCP Chunker Engine\n(Splits ClientHello)"]
        Morph["Frame Morpher\n(Dynamic Length Jitter)"]
        UI --> S5 --> Chunker --> Morph
    end

    subgraph Internet["🌐 Censored Network / TSPU / GFW"]
        Morph -- "TLS 1.3 Reality Camouflage\n(SNI: www.microsoft.com)" --> DPI["DPI Inspection Box"]
    end

    subgraph ServerNode["🛡️ ShadowStar Server (VPS)"]
        direction TB
        Verifier{"HMAC SessionID\nValidator"}
        TunnelCore["SSP Decryption Engine\nAES-256-GCM / ChaCha20"]
        Decoy["Fallback Shield\n200 OK Decoy Proxy"]
        DPI --> Verifier
        Verifier -- "Legit Secret Token" --> TunnelCore --> Target["Internet / Blocked Resources"]
        Verifier -- "Active Probe / Scanner" --> Decoy --> Microsoft["microsoft.com:443"]
    end
```

### ⚡ Key Capabilities
1. **TLS 1.3 Reality Masking**: Traffic is cryptographically indistinguishable from legitimate HTTPS traffic to verified enterprise domains (`www.microsoft.com`, `cloudflare.com`).
2. **TCP Chunker**: Splits TLS `ClientHello` across TCP packet boundaries with randomized micro-delays, defeating SNI inspection without TLS handshake failure.
3. **Dynamic Frame Morphing**: Adds cryptographically pseudo-random padding (16–256 bytes) to every frame, defeating neural-network length-signature classifiers.
4. **Shield Fallback (Anti-Active Probing)**: When censors probe the port, the server transparently proxies the connection to a real decoy domain, returning authentic TLS certificates and HTTP 200 OK.
5. **Multi-Node Manager & Subscriptions**: Supports single `shadowstar://` URIs, Base64 subscription links, JSON configs, concurrent ping tests, and automatic fastest-node selection.

---

### 🖥️ Desktop Client (Windows)
Written in pure Go using native Windows Win32 API and GDI+ (no Chromium, no WebView2, no Electron).
- **Run**: `shadowstar.exe -c shadowstar-client.json`
- **One-Click System Proxy**: Configures Windows WinINet proxy with compatibility checks for AmneziaWG.
- **In-App Updater**: Automatically checks GitHub Releases for new updates.
- **Language Switch**: Click `[RU | EN]` in the header to switch languages instantly.

### 📱 Mobile Client (Android & iOS)
- **Standalone Mobile Gateway**: `bin/shadowstar-mobile.exe` or `bin/shadowstar-mobile-android-arm64`.
- **Progressive Web App (PWA)**: Open `http://<IP>:10810` on your mobile browser and tap **Add to Home Screen**.
- **Android Native**: Ready-to-build Android Studio project in `mobile/android/` with background `ShadowStarVpnService`.

---

### 🚀 Server Quick Start (Linux VPS)
Run one-line automated installation on Ubuntu, Debian, CentOS, or AlmaLinux:
```bash
curl -fsSL https://raw.githubusercontent.com/shadowstar/shadowstar/main/scripts/install.sh -o install.sh && sudo bash install.sh
```
The installer configures:
- Automated system architecture detection (`x86_64` / `aarch64`)
- Linux kernel BBR congestion control optimization
- Systemd service (`shadowstar.service`)
- UFW / Firewalld port configuration
- Generates `shadowstar://` link and console QR code

---

## 🇷🇺 Обзор на русском

**ShadowStar (SSP)** — это высокоскоростной протокол скрытного проксирования нового поколения, разработанный для гарантированного обхода систем глубокой фильтрации трафика (**ТСПУ РКН, GFW, DPI, блокировок по белым спискам и протоколам**) и защиты от активного сетевого сканирования.

### 🌟 Преимущества перед классическими протоколами
| Ограничение стандартных VPN/Shadowsocks | Решение в ShadowStar (SSP) |
| :--- | :--- |
| **Высокая энтропия первого пакета** (DPI блокирует случайные байты) | **TLS 1.3 Reality**: трафик на 100% идентичен настоящему сеансу браузера к доверенному домену (`www.microsoft.com`). |
| **Анализ длин пакетов** (нейросети выявляют размеры рукопожатий) | **Dynamic Frame Morphing**: в каждый кадр подмешивается случайный шум (16–256 байт), полностью скрывая сигнатуру. |
| **Фильтрация по SNI в первом TCP сегменте** | **TCP Chunker**: ClientHello дробится на несколько фрагментов с микрозадержкой. Оборудование ТСПУ пропускает поток без детекта. |
| **Активное зондирование цензоров** (сканирование подозрительных IP) | **Shield Fallback**: если подключение не содержит верного криптографического HMAC-токена, сервер проксирует скан на настоящий сайт Microsoft. Зонд получает ответ 200 OK! |
| **Тяжелые клиенты на Electron** (250+ МБ ОЗУ) | **Pure Win32 GDI+**: сверхбыстрое нативное приложение Windows, потребляющее всего ~15 МБ ОЗУ. |

---

### 🖥️ Настольный клиент для Windows (Desktop Client)
- **Запуск**: запустите `shadowstar.exe` или `run-personal-pc.bat`.
- **Подписки**: автоматическое обновление списка узлов по HTTPS-ссылке (JSON / Base64 / `shadowstar://`).
- **Редактирование и удаление узлов**: изменяйте IP, порт, ключ, SNI и удаляйте ненужные узлы прямо в интерфейсе.
- **Авто-выбор сервера**: кнопка **«⚡ Выбрать самый быстрый»** проверяет задержку всех серверов и подключает лучший.
- **Обновление через GitHub**: кнопка **«⚡ Update»** и пункт меню **«🔄 Проверить обновления»** проверяют свежие релизы и открывают чейнджлог.
- **Двуязычный интерфейс**: переключение языка `[ RU | EN ]` в один клик в шапке окна.

---

### 📱 Мобильная версия (Mobile Gateway & PWA)
- Все веб-ресурсы встроены в бинарный файл (`//go:embed`).
- **Запуск шлюза**: `bin/shadowstar-mobile.exe -c shadowstar-mobile.personal.json`
- Откройте в браузере телефона адрес (например: `http://192.168.1.50:10810`).
- Нажмите **«Добавить на главный экран»** (Add to Home Screen) — приложение установится как нативное PWA с полноэкранным интерфейсом и тактильным откликом.
- В репозитории в каталоге `mobile/android/` доступен полноценный проект Android Studio с сервисом `VpnService`.

---

## 📦 Сборка и релизы (Build & Releases)

### Автоматическая сборка GitHub Releases
В репозитории настроен GitHub Actions Workflow (`.github/workflows/release.yml`), который при пуше тега версии (`v*`) автоматически собирает релизные архивы под все платформы:
- `shadowstar-windows-amd64.zip` — Windows Desktop GUI & Client
- `shadowstar-linux-amd64.tar.gz` — Linux x86_64 CLI Client & Server
- `shadowstar-linux-arm64.tar.gz` — Linux ARM64 CLI Client & Server
- `shadowstar-mobile-android-arm64.tar.gz` — Android ARM64 Gateway
- `shadowstar-mobile-windows-amd64.zip` — Windows Mobile Gateway

### Ручная компиляция:
```powershell
# Сборка десктопного клиента Windows:
go build -ldflags="-s -w" -o bin/shadowstar.exe ./cmd/shadowstar

# Сборка сервера под Linux x86_64:
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/shadowstar-server-linux-amd64 ./cmd/shadowstar-server

# Сборка мобильного шлюза под Android ARM64:
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/shadowstar-mobile-android-arm64 ./cmd/shadowstar-mobile
```

---

## 🔒 Безопасность и приватность (Security & Privacy)
- **Zero-Logs Architecture**: Клиент и сервер не ведут журналов посещенных сайтов и передаваемых данных.
- **Изоляция личных данных**: Конфигурационные файлы личных серверов (`*.personal.json`, `run-personal-*.bat`) строго изолированы и добавлены в `.gitignore`.
- **Шифрование сессий**: AES-256-GCM / ChaCha20-Poly1305 с уникальным эфемерным ключом для каждого TCP-потока.

---

## 📄 Лицензия
Проект распространяется под свободной лицензией [MIT](LICENSE).
