package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/i18n"
)

// RenderFrame renders a complete, flicker-free TUI frame
func (t *TUI) RenderFrame(width, height int) string {
	var buf bytes.Buffer

	// Clamp minimum dimensions
	if width < 72 {
		width = 72
	}
	if height < 20 {
		height = 20
	}

	innerW := width - 2

	// 1. Header Box
	buf.WriteString(t.renderHeader(innerW))
	buf.WriteString("\n")

	// 2. Active Node & Network Dashboard (4 lines)
	buf.WriteString(t.renderDashboard(innerW))
	buf.WriteString("\n")

	// 3. Tab Bar
	buf.WriteString(t.renderTabBar(innerW))
	buf.WriteString("\n")

	// 4. Main Body Content (Servers, Logs, or Diagnostics)
	// Body height is total height minus header(3), dashboard(6), tabs(1), footer(3), borders
	bodyH := height - 14
	if bodyH < 6 {
		bodyH = 6
	}

	switch t.activeTab {
	case TabServers:
		buf.WriteString(t.renderServersList(innerW, bodyH))
	case TabLogs:
		buf.WriteString(t.renderLogs(innerW, bodyH))
	case TabDiagnostics:
		buf.WriteString(t.renderDiagnostics(innerW, bodyH))
	default:
		buf.WriteString(t.renderServersList(innerW, bodyH))
	}
	buf.WriteString("\n")

	// 5. Toast / Notification line
	buf.WriteString(t.renderToastLine(innerW))
	buf.WriteString("\n")

	// 6. Footer Hotkey Bar
	buf.WriteString(t.renderFooter(innerW))

	// 7. Overlay Modal if active
	if t.modal != ModalNone {
		return t.overlayModal(buf.String(), width, height)
	}

	return buf.String()
}

func (t *TUI) renderHeader(w int) string {
	var sb strings.Builder
	sb.WriteString("┌" + strings.Repeat("─", w) + "┐\n")

	// Title + Status Badge + Lang Badge
	title := BrightCyan(Bold("⭐ SHADOWSTAR (SSP)")) + Dim(" • Anti-DPI Stealth Proxy v1.4.0")
	
	statusBadge := ""
	if t.stats.Connected {
		statusBadge = BrightGreen(Bold("[● CONNECTED]"))
	} else {
		statusBadge = Gray("[○ DISCONNECTED]")
	}

	langBadge := Cyan("[ RU | EN ]")
	if i18n.CurrentLang == i18n.LangRU {
		langBadge = Cyan("[") + BrightYellow(Bold("RU")) + Cyan("|EN]")
	} else {
		langBadge = Cyan("[RU|") + BrightYellow(Bold("EN")) + Cyan("]")
	}

	content := fmt.Sprintf(" %s   %s   %s", title, statusBadge, langBadge)
	pad := w - VisibleLen(content)
	if pad < 0 {
		pad = 0
	}
	sb.WriteString("│" + content + strings.Repeat(" ", pad) + "│\n")
	sb.WriteString("├" + strings.Repeat("─", w) + "┤")
	return sb.String()
}

func (t *TUI) renderDashboard(w int) string {
	var sb strings.Builder

	active := t.getActiveServer()

	// Line 1: Active server name, address, latency
	latencyStr := Gray("--- ms")
	if t.stats.Latency > 0 {
		if t.stats.Latency < 60 {
			latencyStr = BrightGreen(fmt.Sprintf("%d ms", t.stats.Latency))
		} else if t.stats.Latency < 120 {
			latencyStr = BrightYellow(fmt.Sprintf("%d ms", t.stats.Latency))
		} else {
			latencyStr = BrightRed(fmt.Sprintf("%d ms", t.stats.Latency))
		}
	}

	countryStr := ""
	if active.Country != "" {
		countryStr = fmt.Sprintf("[%s] ", active.Country)
	}

	l1Left := fmt.Sprintf(" %s %s%s (%s)",
		Yellow("★ "+i18n.T("Активный узел:", "Active Node:")),
		BrightCyan(Bold(countryStr+active.Name)),
		Gray(""),
		Cyan(active.ServerAddr),
	)
	l1Right := fmt.Sprintf("%s: %s ", i18n.T("Задержка", "Latency"), latencyStr)
	sb.WriteString(t.formatLine(l1Left, l1Right, w))
	sb.WriteString("\n")

	// Line 2: Camouflage SNI, Chunker, System Proxy
	chunkerStr := Gray("[DISABLED]")
	if active.EnableChunker {
		chunkerStr = BrightGreen("[ENABLED]")
	}

	sysProxyStr := Gray("[OFF]")
	if t.sysProxyStatus {
		sysProxyStr = BrightGreen("[ACTIVE]")
	}

	l2Left := fmt.Sprintf(" %s TLS 1.3 Reality (%s) | Chunker: %s",
		Magenta("🛡️ "+i18n.T("Защита:", "Shield:")),
		BrightCyan(active.SNI),
		chunkerStr,
	)
	l2Right := fmt.Sprintf("%s: %s ", i18n.T("Системный прокси", "System Proxy"), sysProxyStr)
	sb.WriteString(t.formatLine(l2Left, l2Right, w))
	sb.WriteString("\n")

	// Line 3: Live Speeds & Sparklines
	sparkW := 14
	if w > 90 {
		sparkW = 20
	}
	upSpark := RenderSparkline(t.historyUp, 0, sparkW)
	downSpark := RenderSparkline(t.historyDown, 0, sparkW)

	l3Left := fmt.Sprintf(" ▲ UP: %s %s   ▼ DOWN: %s %s",
		BrightYellow(PadRight(FormatSpeed(t.stats.SpeedUp), 10)),
		Yellow(upSpark),
		BrightGreen(PadRight(FormatSpeed(t.stats.SpeedDown), 10)),
		Green(downSpark),
	)

	upTimeStr := FormatDuration(0)
	if t.stats.Connected && t.stats.ConnectedTime > 0 {
		upTimeStr = FormatDuration(time.Now().Unix() - t.stats.ConnectedTime)
	}
	l3Right := fmt.Sprintf("%s: %s ", i18n.T("Время", "Uptime"), Cyan(upTimeStr))
	sb.WriteString(t.formatLine(l3Left, l3Right, w))
	sb.WriteString("\n")

	// Line 4: Inbounds & Total Traffic
	l4Left := fmt.Sprintf(" %s SOCKS5 %s | HTTP %s",
		Cyan("📡 "+i18n.T("Шлюз:", "Inbound:")),
		BrightCyan(t.cfg.SocksAddr),
		BrightCyan(t.cfg.HttpAddr),
	)
	l4Right := fmt.Sprintf("▲ %s  ▼ %s  (%s: %d) ",
		FormatBytes(t.stats.BytesUp),
		FormatBytes(t.stats.BytesDown),
		i18n.T("Потоков", "Streams"),
		t.stats.ActiveConns,
	)
	sb.WriteString(t.formatLine(l4Left, l4Right, w))

	return sb.String()
}

func (t *TUI) renderTabBar(w int) string {
	var sb strings.Builder
	sb.WriteString("├" + strings.Repeat("─", w) + "┤\n")

	tab1 := i18n.T(" [1] Серверы ", " [1] Servers ")
	tab2 := i18n.T(" [2] Логи туннеля ", " [2] Tunnel Logs ")
	tab3 := i18n.T(" [3] Диагностика ", " [3] Diagnostics ")

	if t.activeTab == TabServers {
		tab1 = InverseSeq + BoldSeq + tab1 + Reset
	} else {
		tab1 = Dim(tab1)
	}

	if t.activeTab == TabLogs {
		tab2 = InverseSeq + BoldSeq + tab2 + Reset
	} else {
		tab2 = Dim(tab2)
	}

	if t.activeTab == TabDiagnostics {
		tab3 = InverseSeq + BoldSeq + tab3 + Reset
	} else {
		tab3 = Dim(tab3)
	}

	left := fmt.Sprintf(" %s %s %s", tab1, tab2, tab3)
	right := Gray(fmt.Sprintf("%d "+i18n.T("узлов", "nodes")+" ", len(t.cfg.Servers)))
	sb.WriteString(t.formatLine(left, right, w))
	sb.WriteString("\n├" + strings.Repeat("─", w) + "┤")
	return sb.String()
}

func (t *TUI) renderServersList(w, h int) string {
	var sb strings.Builder

	servers := t.cfg.Servers
	if len(servers) == 0 {
		emptyMsg := i18n.T("Список серверов пуст. Нажмите [A] чтобы добавить узел или [U] для подписки.",
			"No servers configured. Press [A] to add a node or [U] for subscription.")
		for i := 0; i < h; i++ {
			if i == h/2 {
				sb.WriteString("│" + Center(Yellow(emptyMsg), w) + "│\n")
			} else {
				sb.WriteString("│" + strings.Repeat(" ", w) + "│\n")
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	// Adjust selection bounds
	if t.selectedIdx < 0 {
		t.selectedIdx = 0
	}
	if t.selectedIdx >= len(servers) {
		t.selectedIdx = len(servers) - 1
	}

	// Adjust scroll window
	if t.selectedIdx < t.scrollOffset {
		t.scrollOffset = t.selectedIdx
	}
	if t.selectedIdx >= t.scrollOffset+h {
		t.scrollOffset = t.selectedIdx - h + 1
	}

	// Render rows
	for i := 0; i < h; i++ {
		idx := t.scrollOffset + i
		if idx >= len(servers) {
			sb.WriteString("│" + strings.Repeat(" ", w) + "│\n")
			continue
		}

		s := servers[idx]
		isSelected := (idx == t.selectedIdx)
		isActive := (s.ID == t.cfg.ActiveID)

		// Status bullet
		statusMark := "○"
		if isActive {
			statusMark = "●"
		}

		// Country tag
		countryTag := "[--]"
		if s.Country != "" {
			countryTag = fmt.Sprintf("[%s]", s.Country)
		}

		// Ping latency display
		pingStr := "   -- ms"
		if s.Ping > 0 {
			if s.Ping < 60 {
				pingStr = BrightGreen(fmt.Sprintf("%4d ms", s.Ping))
			} else if s.Ping < 120 {
				pingStr = BrightYellow(fmt.Sprintf("%4d ms", s.Ping))
			} else {
				pingStr = BrightRed(fmt.Sprintf("%4d ms", s.Ping))
			}
		}

		prefix := "  "
		if isSelected {
			prefix = BrightCyan("▶ ")
		}

		activeTag := ""
		if isActive {
			activeTag = BrightGreen(" [" + i18n.T("АКТИВЕН", "ACTIVE") + "]")
		}

		nameColW := 26
		if w > 90 {
			nameColW = 34
		}
		nameStr := PadRight(Truncate(s.Name, nameColW), nameColW)

		addrColW := 22
		if w > 90 {
			addrColW = 26
		}
		addrStr := PadRight(Truncate(s.ServerAddr, addrColW), addrColW)

		sniColW := 18
		sniStr := PadRight(Truncate(s.SNI, sniColW), sniColW)

		lineContent := fmt.Sprintf("%s%s %s %s  %s  %s  %s%s",
			prefix,
			statusMark,
			Cyan(countryTag),
			BrightCyan(nameStr),
			Gray(addrStr),
			Dim(sniStr),
			pingStr,
			activeTag,
		)

		if isSelected {
			// Highlight selected row with background or bold
			lineContent = Bold(lineContent)
		}

		pad := w - VisibleLen(lineContent)
		if pad < 0 {
			pad = 0
		}
		sb.WriteString("│" + lineContent + strings.Repeat(" ", pad) + "│\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (t *TUI) renderLogs(w, h int) string {
	var sb strings.Builder

	logs := t.client.GetLogs()
	if len(logs) == 0 {
		for i := 0; i < h; i++ {
			if i == h/2 {
				sb.WriteString("│" + Center(Gray(i18n.T("Логи пока пусты...", "No connection logs yet...")), w) + "│\n")
			} else {
				sb.WriteString("│" + strings.Repeat(" ", w) + "│\n")
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	start := 0
	if len(logs) > h {
		start = len(logs) - h
	}
	visibleLogs := logs[start:]

	for i := 0; i < h; i++ {
		if i >= len(visibleLogs) {
			sb.WriteString("│" + strings.Repeat(" ", w) + "│\n")
			continue
		}
		logLine := visibleLogs[i]

		// Colorize log prefixes
		if strings.Contains(logLine, "[Handshake-Success]") || strings.Contains(logLine, "[Auth-OK]") {
			logLine = BrightGreen(logLine)
		} else if strings.Contains(logLine, "[Warning]") || strings.Contains(logLine, "[Probe-Shield]") {
			logLine = BrightYellow(logLine)
		} else if strings.Contains(logLine, "[Error]") || strings.Contains(logLine, "failed") {
			logLine = BrightRed(logLine)
		} else if strings.Contains(logLine, "[Handshake]") || strings.Contains(logLine, "[Tunnel]") {
			logLine = BrightCyan(logLine)
		} else {
			logLine = Gray(logLine)
		}

		line := " " + Truncate(logLine, w-2)
		pad := w - VisibleLen(line)
		if pad < 0 {
			pad = 0
		}
		sb.WriteString("│" + line + strings.Repeat(" ", pad) + "│\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (t *TUI) renderDiagnostics(w, h int) string {
	var sb strings.Builder

	active := t.getActiveServer()

	lines := []string{
		fmt.Sprintf(" %s", Bold(Cyan(i18n.T("--- ДИАГНОСТИКА СТЕК ПРОТОКОЛА SHADOWSTAR (SSP) ---", "--- SHADOWSTAR (SSP) PROTOCOL DIAGNOSTICS ---")))),
		"",
		fmt.Sprintf("  • %-24s: %s", i18n.T("Криптографический шифр", "Cipher Suite"), "AES-256-GCM / ChaCha20-Poly1305 (AEAD)"),
		fmt.Sprintf("  • %-24s: %s", i18n.T("Обмен ключами", "Key Exchange"), "X25519 (ECDH) + HKDF-SHA256 Derivation"),
		fmt.Sprintf("  • %-24s: %s", i18n.T("Маскировка рукопожатия", "Handshake Camouflage"), fmt.Sprintf("TLS 1.3 Reality (SNI: %s)", active.SNI)),
		fmt.Sprintf("  • %-24s: %s", i18n.T("Аутентификация сессии", "Session Authentication"), "Embedded Replay-Protected HMAC in TLS SessionID (32-byte)"),
		fmt.Sprintf("  • %-24s: %s", i18n.T("Защита от активных зондов", "Active Probe Defense"), "Shield Fallback -> 200 OK Decoy Proxy"),
		fmt.Sprintf("  • %-24s: %s", i18n.T("Фрагментация TCP Chunker", "TCP Chunker Fragmentation"), fmt.Sprintf("%v (ClientHello Split + 2ms jitter)", active.EnableChunker)),
		fmt.Sprintf("  • %-24s: %s", i18n.T("Динамический шум кадров", "Frame Morphing Noise"), fmt.Sprintf("%d bytes dynamic pseudo-random padding", active.PaddingLen)),
		"",
		fmt.Sprintf(" %s", Bold(Cyan(i18n.T("--- ЛОКАЛЬНЫЕ СЕТЕВЫЕ ИНТЕРФЕЙСЫ ---", "--- LOCAL NETWORK INTERFACES ---")))),
		fmt.Sprintf("  • SOCKS5 Inbound         : %s (TCP + UDP Associate)", t.cfg.SocksAddr),
		fmt.Sprintf("  • HTTP Connect Inbound   : %s", t.cfg.HttpAddr),
		fmt.Sprintf("  • WinINet System Proxy   : %v", t.sysProxyStatus),
	}

	for i := 0; i < h; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		pad := w - VisibleLen(line)
		if pad < 0 {
			pad = 0
		}
		sb.WriteString("│" + line + strings.Repeat(" ", pad) + "│\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (t *TUI) renderToastLine(w int) string {
	var sb strings.Builder
	sb.WriteString("├" + strings.Repeat("─", w) + "┤\n")

	msg := ""
	if t.toastTimer > 0 && t.toastMsg != "" {
		msg = " ⚡ " + BrightYellow(Bold(t.toastMsg))
	} else if t.isPingingAll {
		msg = " ⌛ " + BrightCyan(i18n.T("Проверка задержки серверов...", "Pinging all servers concurrently..."))
	} else if t.isUpdatingSub {
		msg = " ⌛ " + BrightCyan(i18n.T("Обновление подписки...", "Fetching subscription updates..."))
	}

	pad := w - VisibleLen(msg)
	if pad < 0 {
		pad = 0
	}
	sb.WriteString("│" + msg + strings.Repeat(" ", pad) + "│\n")
	sb.WriteString("├" + strings.Repeat("─", w) + "┤")
	return sb.String()
}

func (t *TUI) renderFooter(w int) string {
	var sb strings.Builder

	cKey := BrightCyan("[C]")
	cText := i18n.T("Подкл/Откл", "Connect/Disconnect")

	pKey := BrightCyan("[P]")
	pText := i18n.T("Пинг", "Ping All")

	fKey := BrightCyan("[F]")
	fText := i18n.T("Быстрый", "Fastest")

	sKey := BrightCyan("[S]")
	sText := i18n.T("Прокси", "SysProxy")

	aKey := BrightCyan("[A]")
	aText := i18n.T("Добавить", "Add Node")

	dKey := BrightCyan("[D]")
	dText := i18n.T("Удалить", "Delete")

	uKey := BrightCyan("[U]")
	uText := i18n.T("Подписка", "Sub")

	tabKey := BrightCyan("[Tab]")
	tabText := i18n.T("Язык", "Lang")

	qKey := BrightCyan("[Q]")
	qText := i18n.T("Выход", "Quit")

	line := fmt.Sprintf(" %s %s %s %s %s %s %s %s %s %s %s %s %s %s %s %s %s %s",
		cKey, cText,
		fKey, fText,
		pKey, pText,
		sKey, sText,
		aKey, aText,
		dKey, dText,
		uKey, uText,
		tabKey, tabText,
		qKey, qText,
	)

	pad := w - VisibleLen(line)
	if pad < 0 {
		pad = 0
	}
	sb.WriteString("│" + line + strings.Repeat(" ", pad) + "│\n")
	sb.WriteString("└" + strings.Repeat("─", w) + "┘")
	return sb.String()
}

func (t *TUI) formatLine(left, right string, w int) string {
	lLen := VisibleLen(left)
	rLen := VisibleLen(right)
	pad := w - lLen - rLen
	if pad < 0 {
		pad = 0
	}
	return "│" + left + strings.Repeat(" ", pad) + right + "│"
}

func (t *TUI) overlayModal(bg string, totalW, totalH int) string {
	lines := strings.Split(bg, "\n")
	for len(lines) < totalH {
		lines = append(lines, strings.Repeat(" ", totalW))
	}

	modalW := 62
	if modalW > totalW-4 {
		modalW = totalW - 4
	}
	modalH := 9
	startRow := (totalH - modalH) / 2
	startCol := (totalW - modalW) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	modalLines := make([]string, modalH)
	modalInnerW := modalW - 2

	switch t.modal {
	case ModalAddNode:
		modalLines[0] = "┌" + strings.Repeat("─", modalInnerW) + "┐"
		modalLines[1] = "│" + Center(BrightCyan(Bold(i18n.T("ДОБАВЛЕНИЕ УЗЛА (SHADOWSTAR)", "ADD SHADOWSTAR NODE"))), modalInnerW) + "│"
		modalLines[2] = "├" + strings.Repeat("─", modalInnerW) + "┤"
		modalLines[3] = "│ " + PadRight(i18n.T("Вставьте shadowstar:// ссылку или host:port:key:", "Paste shadowstar:// URI or host:port:key:"), modalInnerW-1) + "│"
		inputStr := t.modalInput + "_"
		modalLines[4] = "│ " + PadRight(BrightYellow(Truncate(inputStr, modalInnerW-2)), modalInnerW-1) + "│"
		errMsg := ""
		if t.modalError != "" {
			errMsg = BrightRed(t.modalError)
		}
		modalLines[5] = "│ " + PadRight(errMsg, modalInnerW-1) + "│"
		modalLines[6] = "├" + strings.Repeat("─", modalInnerW) + "┤"
		modalLines[7] = "│" + Center(Gray(i18n.T("[Enter] Добавить  |  [Esc] Отмена", "[Enter] Submit  |  [Esc] Cancel")), modalInnerW) + "│"
		modalLines[8] = "└" + strings.Repeat("─", modalInnerW) + "┘"

	case ModalSubURL:
		modalLines[0] = "┌" + strings.Repeat("─", modalInnerW) + "┐"
		modalLines[1] = "│" + Center(BrightCyan(Bold(i18n.T("URL ПОДПИСКИ СЕРВЕРОВ", "SERVER SUBSCRIPTION URL"))), modalInnerW) + "│"
		modalLines[2] = "├" + strings.Repeat("─", modalInnerW) + "┤"
		modalLines[3] = "│ " + PadRight(i18n.T("Введите HTTPS URL подписки (JSON / Base64):", "Enter HTTPS subscription URL (JSON / Base64):"), modalInnerW-1) + "│"
		inputStr := t.modalInput + "_"
		modalLines[4] = "│ " + PadRight(BrightYellow(Truncate(inputStr, modalInnerW-2)), modalInnerW-1) + "│"
		errMsg := ""
		if t.modalError != "" {
			errMsg = BrightRed(t.modalError)
		}
		modalLines[5] = "│ " + PadRight(errMsg, modalInnerW-1) + "│"
		modalLines[6] = "├" + strings.Repeat("─", modalInnerW) + "┤"
		modalLines[7] = "│" + Center(Gray(i18n.T("[Enter] Сохранить и обновить  |  [Esc] Отмена", "[Enter] Save & Fetch  |  [Esc] Cancel")), modalInnerW) + "│"
		modalLines[8] = "└" + strings.Repeat("─", modalInnerW) + "┘"

	case ModalConfirmDelete:
		active := t.getActiveServer()
		modalLines[0] = "┌" + strings.Repeat("─", modalInnerW) + "┐"
		modalLines[1] = "│" + Center(BrightRed(Bold(i18n.T("УДАЛЕНИЕ УЗЛА", "DELETE NODE"))), modalInnerW) + "│"
		modalLines[2] = "├" + strings.Repeat("─", modalInnerW) + "┤"
		modalLines[3] = "│" + Center(i18n.T("Удалить выбранный сервер?", "Delete currently selected server?"), modalInnerW) + "│"
		modalLines[4] = "│" + Center(BrightYellow(active.Name), modalInnerW) + "│"
		modalLines[5] = "│" + Center(Cyan(active.ServerAddr), modalInnerW) + "│"
		modalLines[6] = "├" + strings.Repeat("─", modalInnerW) + "┤"
		modalLines[7] = "│" + Center(i18n.T("[Y] Удалить  |  [N/Esc] Отмена", "[Y] Delete  |  [N/Esc] Cancel"), modalInnerW) + "│"
		modalLines[8] = "└" + strings.Repeat("─", modalInnerW) + "┘"
	}

	// Splice modal into background
	for r := 0; r < modalH; r++ {
		rowIdx := startRow + r
		if rowIdx >= len(lines) {
			break
		}
		bgRow := lines[rowIdx]
		// Clean ANSI from bg row to slice cleanly
		cleanBg := StripANSI(bgRow)
		leftPart := ""
		if startCol < len([]rune(cleanBg)) {
			leftPart = string([]rune(cleanBg)[:startCol])
		} else {
			leftPart = cleanBg + strings.Repeat(" ", startCol-len([]rune(cleanBg)))
		}

		rightPart := ""
		rightStart := startCol + modalW
		if rightStart < len([]rune(cleanBg)) {
			rightPart = string([]rune(cleanBg)[rightStart:])
		}

		lines[rowIdx] = leftPart + modalLines[r] + rightPart
	}

	return strings.Join(lines, "\n")
}

func (t *TUI) getActiveServer() client.ServerProfile {
	for _, s := range t.cfg.Servers {
		if s.ID == t.cfg.ActiveID {
			return s
		}
	}
	if len(t.cfg.Servers) > 0 {
		return t.cfg.Servers[0]
	}
	return client.ServerProfile{
		Name:       "None",
		ServerAddr: "0.0.0.0:0",
		SNI:        "www.microsoft.com",
	}
}
