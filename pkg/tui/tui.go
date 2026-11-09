package tui

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/config"
	"github.com/shadowstar/shadowstar/pkg/i18n"
	"github.com/shadowstar/shadowstar/pkg/sysproxy"
)

const (
	TabServers     = 0
	TabLogs        = 1
	TabDiagnostics = 2
)

const (
	ModalNone = iota
	ModalAddNode
	ModalSubURL
	ModalConfirmDelete
	ModalAddGroup
	ModalEditGroup
	ModalDeleteGroup
)

type TUI struct {
	client     *client.Client
	cfg        *client.Config
	cfgPath    string
	activeTab  int
	selectedIdx int
	scrollOffset int

	// Modals & User input
	modal         int
	modalStep     int
	tempGroupName string
	modalInput    string
	modalError    string
	toastMsg      string
	toastTimer    int

	// Telemetry & metrics
	stats          client.Stats
	historyDown    []float64
	historyUp      []float64
	sysProxyStatus bool
	isPingingAll   bool
	isUpdatingSub  bool

	mu sync.Mutex
}

func New(cfgPath string, c *client.Client, cfg *client.Config) *TUI {
	cfg.EnsureGroups()
	t := &TUI{
		client:         c,
		cfg:            cfg,
		cfgPath:        cfgPath,
		activeTab:      TabServers,
		selectedIdx:    0,
		historyDown:    make([]float64, 0, 40),
		historyUp:      make([]float64, 0, 40),
		sysProxyStatus: sysproxy.GetSystemProxyStatus(),
	}

	// Match initial selection to active server within active group
	activeGroupServers := cfg.GetServersInGroup(cfg.ActiveGroupID)
	for i, s := range activeGroupServers {
		if s.ID == cfg.ActiveID {
			t.selectedIdx = i
			break
		}
	}

	return t
}

func (t *TUI) SetStateForTest(stats client.Stats, toast string, histUp, histDown []float64) {
	t.stats = stats
	t.toastMsg = toast
	t.toastTimer = 10
	t.historyUp = histUp
	t.historyDown = histDown
}

func (t *TUI) Run(autoConnect bool) error {
	if err := InitTerminal(); err != nil {
		return err
	}
	defer RestoreTerminal()

	// Handle SIGINT / SIGTERM for clean terminal and proxy reset
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		t.cleanupAndExit()
	}()

	if autoConnect {
		if err := t.client.Start(); err != nil {
			t.toast(i18n.T("Ошибка авто-подключения: ", "Auto-connect failed: ") + err.Error())
		} else {
			t.toast(i18n.T("✓ Автоматически подключено!", "✓ Connected successfully!"))
		}
	}

	keyChan := make(chan KeyEvent, 10)
	go t.readKeysLoop(keyChan)

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	// Initial render
	t.render()

	for {
		select {
		case key := <-keyChan:
			if shouldExit := t.handleKey(key); shouldExit {
				t.cleanupAndExit()
				return nil
			}
			t.render()

		case <-ticker.C:
			t.updateTelemetry()
			t.render()
		}
	}
}

func (t *TUI) updateTelemetry() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.stats = t.client.GetStats()
	t.sysProxyStatus = sysproxy.GetSystemProxyStatus()

	// Keep rolling history of last 30 speed values
	t.historyDown = append(t.historyDown, t.stats.SpeedDown)
	if len(t.historyDown) > 30 {
		t.historyDown = t.historyDown[1:]
	}

	t.historyUp = append(t.historyUp, t.stats.SpeedUp)
	if len(t.historyUp) > 30 {
		t.historyUp = t.historyUp[1:]
	}

	if t.toastTimer > 0 {
		t.toastTimer--
		if t.toastTimer == 0 {
			t.toastMsg = ""
		}
	}
}

func (t *TUI) render() {
	t.mu.Lock()
	w, h := GetTerminalSize()
	frame := t.RenderFrame(w, h)
	t.mu.Unlock()

	// Move cursor to home (top-left) and draw complete frame in a single write call
	os.Stdout.WriteString("\x1b[H" + frame)
}

func (t *TUI) readKeysLoop(out chan<- KeyEvent) {
	kr := NewKeyReader(os.Stdin)
	for {
		ke, err := kr.ReadKey()
		if err != nil {
			return
		}
		out <- ke
	}
}

func (t *TUI) handleKey(key KeyEvent) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Ctrl+C always exits
	if key.Type == KeyCtrlC {
		return true
	}

	// 1. Modal input mode
	if t.modal != ModalNone {
		return t.handleModalKey(key)
	}

	// 2. Main screen shortcuts
	switch key.Type {
	case KeyEsc:
		return true

	case KeyTab:
		i18n.ToggleLang()
		return false

	case KeyLeft:
		t.prevGroup()
		return false

	case KeyRight:
		t.nextGroup()
		return false

	case KeyUp:
		if t.selectedIdx > 0 {
			t.selectedIdx--
		}
		return false

	case KeyDown:
		servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
		if t.selectedIdx < len(servers)-1 {
			t.selectedIdx++
		}
		return false

	case KeyPageUp:
		t.selectedIdx -= 5
		if t.selectedIdx < 0 {
			t.selectedIdx = 0
		}
		return false

	case KeyPageDown:
		servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
		t.selectedIdx += 5
		if t.selectedIdx >= len(servers) {
			t.selectedIdx = len(servers) - 1
		}
		if t.selectedIdx < 0 {
			t.selectedIdx = 0
		}
		return false

	case KeyHome:
		t.selectedIdx = 0
		return false

	case KeyEnd:
		servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
		if len(servers) > 0 {
			t.selectedIdx = len(servers) - 1
		}
		return false

	case KeyEnter:
		t.selectOrConnectActive()
		return false

	case KeyDelete:
		servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
		if len(servers) > 0 {
			t.modal = ModalConfirmDelete
		}
		return false

	case KeyRune:
		switch key.Rune {
		case 'q', 'Q':
			return true

		case 'c', 'C', ' ':
			t.toggleConnection()

		case 's', 'S':
			t.toggleSysProxy()

		case 'p', 'P':
			t.pingAllServers()

		case 'f', 'F':
			t.selectFastestServer()

		case 'a', 'A':
			t.modal = ModalAddNode
			t.modalInput = ""
			t.modalError = ""

		case 'n', 'N':
			t.modal = ModalAddGroup
			t.modalStep = 0
			t.modalInput = ""
			t.tempGroupName = ""
			t.modalError = ""

		case 'e', 'E':
			grp := t.cfg.GetActiveGroup()
			if grp != nil {
				t.modal = ModalEditGroup
				t.modalStep = 0
				t.modalInput = grp.Name
				t.tempGroupName = ""
				t.modalError = ""
			}

		case 'x', 'X':
			if len(t.cfg.Groups) <= 1 {
				t.toast(i18n.T("Нельзя удалить единственную категорию!", "Cannot delete the only remaining category!"))
			} else {
				t.modal = ModalDeleteGroup
				t.modalError = ""
			}

		case 'u', 'U':
			grp := t.cfg.GetActiveGroup()
			if grp != nil && grp.URL != "" {
				t.fetchGroupSubscription(grp.ID)
			} else {
				grpName := ""
				if grp != nil {
					grpName = grp.Name
				}
				t.modal = ModalEditGroup
				t.modalStep = 1
				t.modalInput = ""
				t.tempGroupName = grpName
				t.modalError = ""
			}

		case 'd', 'D':
			servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
			if len(servers) > 0 {
				t.modal = ModalConfirmDelete
			}

		case 'r', 'R':
			i18n.ToggleLang()

		case '1':
			t.activeTab = TabServers
		case '2':
			t.activeTab = TabLogs
		case '3':
			t.activeTab = TabDiagnostics

		case 'k':
			if t.selectedIdx > 0 {
				t.selectedIdx--
			}
		case 'j':
			servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
			if t.selectedIdx < len(servers)-1 {
				t.selectedIdx++
			}
		case 'h':
			t.prevGroup()
		case 'l':
			t.nextGroup()
		}
	}

	return false
}

func (t *TUI) nextGroup() {
	if len(t.cfg.Groups) <= 1 {
		return
	}
	curIdx := 0
	for i, g := range t.cfg.Groups {
		if g.ID == t.cfg.ActiveGroupID {
			curIdx = i
			break
		}
	}
	nextIdx := (curIdx + 1) % len(t.cfg.Groups)
	t.cfg.ActiveGroupID = t.cfg.Groups[nextIdx].ID
	t.selectedIdx = 0
	t.scrollOffset = 0
	_ = config.SaveClientConfig(t.cfgPath, t.cfg)
}

func (t *TUI) prevGroup() {
	if len(t.cfg.Groups) <= 1 {
		return
	}
	curIdx := 0
	for i, g := range t.cfg.Groups {
		if g.ID == t.cfg.ActiveGroupID {
			curIdx = i
			break
		}
	}
	prevIdx := (curIdx - 1 + len(t.cfg.Groups)) % len(t.cfg.Groups)
	t.cfg.ActiveGroupID = t.cfg.Groups[prevIdx].ID
	t.selectedIdx = 0
	t.scrollOffset = 0
	_ = config.SaveClientConfig(t.cfgPath, t.cfg)
}

func (t *TUI) handleModalKey(key KeyEvent) bool {
	if key.Type == KeyEsc {
		t.modal = ModalNone
		t.modalInput = ""
		t.modalError = ""
		t.modalStep = 0
		t.tempGroupName = ""
		return false
	}

	if t.modal == ModalDeleteGroup {
		if (key.Type == KeyRune && (key.Rune == 'y' || key.Rune == 'Y')) || key.Type == KeyEnter {
			grp := t.cfg.GetActiveGroup()
			if grp != nil {
				delName := grp.Name
				t.cfg.DeleteGroup(grp.ID)
				_ = config.SaveClientConfig(t.cfgPath, t.cfg)
				t.selectedIdx = 0
				t.scrollOffset = 0
				t.toast(fmt.Sprintf(i18n.T("✓ Категория «%s» удалена", "✓ Category «%s» deleted"), delName))
			}
			t.modal = ModalNone
		} else if key.Type == KeyRune && (key.Rune == 'n' || key.Rune == 'N') {
			t.modal = ModalNone
		}
		return false
	}

	if t.modal == ModalAddGroup {
		switch key.Type {
		case KeyBackspace:
			if len(t.modalInput) > 0 {
				t.modalInput = t.modalInput[:len(t.modalInput)-1]
			}
		case KeyEnter:
			if t.modalStep == 0 {
				name := strings.TrimSpace(t.modalInput)
				if name == "" {
					t.modalError = i18n.T("Название не может быть пустым!", "Name cannot be empty!")
					return false
				}
				t.tempGroupName = name
				t.modalStep = 1
				t.modalInput = ""
				t.modalError = ""
			} else {
				subURL := strings.TrimSpace(t.modalInput)
				grp := t.cfg.AddGroup(t.tempGroupName, subURL)
				_ = config.SaveClientConfig(t.cfgPath, t.cfg)
				t.modal = ModalNone
				t.modalInput = ""
				t.tempGroupName = ""
				t.modalStep = 0
				t.selectedIdx = 0
				t.scrollOffset = 0
				if subURL != "" {
					t.fetchGroupSubscription(grp.ID)
				} else {
					t.toast(fmt.Sprintf(i18n.T("✓ Создана группа «%s»", "✓ Created group «%s»"), grp.Name))
				}
			}
		case KeyRune:
			t.modalInput += string(key.Rune)
		}
		return false
	}

	if t.modal == ModalEditGroup {
		switch key.Type {
		case KeyBackspace:
			if len(t.modalInput) > 0 {
				t.modalInput = t.modalInput[:len(t.modalInput)-1]
			}
		case KeyEnter:
			if t.modalStep == 0 {
				name := strings.TrimSpace(t.modalInput)
				if name == "" {
					t.modalError = i18n.T("Название не может быть пустым!", "Name cannot be empty!")
					return false
				}
				t.tempGroupName = name
				t.modalStep = 1
				curGrp := t.cfg.GetActiveGroup()
				if curGrp != nil {
					t.modalInput = curGrp.URL
				} else {
					t.modalInput = ""
				}
				t.modalError = ""
			} else {
				subURL := strings.TrimSpace(t.modalInput)
				curGrp := t.cfg.GetActiveGroup()
				if curGrp != nil {
					oldURL := curGrp.URL
					t.cfg.UpdateGroup(curGrp.ID, t.tempGroupName, subURL)
					_ = config.SaveClientConfig(t.cfgPath, t.cfg)
					t.modal = ModalNone
					t.modalInput = ""
					t.tempGroupName = ""
					t.modalStep = 0
					t.toast(fmt.Sprintf(i18n.T("✓ Категория «%s» обновлена", "✓ Category «%s» updated"), curGrp.Name))
					if subURL != "" && subURL != oldURL {
						t.fetchGroupSubscription(curGrp.ID)
					}
				}
			}
		case KeyRune:
			t.modalInput += string(key.Rune)
		}
		return false
	}

	if t.modal == ModalConfirmDelete {
		if key.Type == KeyRune && (key.Rune == 'y' || key.Rune == 'Y') || key.Type == KeyEnter {
			servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
			if len(servers) > 0 && t.selectedIdx >= 0 && t.selectedIdx < len(servers) {
				deleted := servers[t.selectedIdx]
				var remaining []client.ServerProfile
				for _, s := range t.cfg.Servers {
					if s.ID != deleted.ID {
						remaining = append(remaining, s)
					}
				}
				t.cfg.Servers = remaining
				newInGroup := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
				if t.selectedIdx >= len(newInGroup) && t.selectedIdx > 0 {
					t.selectedIdx--
				}
				if len(newInGroup) > 0 && deleted.ID == t.cfg.ActiveID {
					t.cfg.ActiveID = newInGroup[t.selectedIdx].ID
					t.client.UpdateProfile(newInGroup[t.selectedIdx])
				}
				_ = config.SaveClientConfig(t.cfgPath, t.cfg)
				t.toast(fmt.Sprintf(i18n.T("✓ Узел удален: %s", "✓ Node deleted: %s"), deleted.Name))
			}
			t.modal = ModalNone
		} else if key.Type == KeyRune && (key.Rune == 'n' || key.Rune == 'N') {
			t.modal = ModalNone
		}
		return false
	}

	// Text input modals: ModalAddNode / ModalSubURL
	switch key.Type {
	case KeyBackspace:
		if len(t.modalInput) > 0 {
			t.modalInput = t.modalInput[:len(t.modalInput)-1]
		}
	case KeyEnter:
		if t.modal == ModalAddNode {
			t.submitAddNode()
		} else if t.modal == ModalSubURL {
			t.submitSubURL()
		}
	case KeyRune:
		t.modalInput += string(key.Rune)
	}

	return false
}

func (t *TUI) submitAddNode() {
	input := strings.TrimSpace(t.modalInput)
	if input == "" {
		t.modalError = i18n.T("Введите данные узла!", "Input cannot be empty!")
		return
	}

	// Try decoding as shadowstar:// URI
	profile, err := config.DecodeURIToProfile(input)
	if err != nil {
		// Try parsing as host:port:key format
		parts := strings.Split(input, ":")
		if len(parts) >= 3 {
			p := client.ServerProfile{
				ID:            fmt.Sprintf("node-%d", time.Now().UnixNano()%10000),
				Name:          "Node " + parts[0],
				ServerAddr:    parts[0] + ":" + parts[1],
				MasterKey:     parts[2],
				SNI:           "www.microsoft.com",
				EnableChunker: true,
				PaddingLen:    64,
			}
			if len(parts) >= 4 {
				p.SNI = parts[3]
			}
			profile = &p
			err = nil
		}
	}

	if err != nil || profile == nil {
		t.modalError = i18n.T("Неверный формат ссылки/ключа", "Invalid link or key format")
		return
	}

	profile.GroupID = t.cfg.ActiveGroupID
	t.cfg.Servers = append(t.cfg.Servers, *profile)
	t.cfg.ActiveID = profile.ID
	t.client.UpdateProfile(*profile)
	servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
	t.selectedIdx = len(servers) - 1
	_ = config.SaveClientConfig(t.cfgPath, t.cfg)

	t.toast(fmt.Sprintf(i18n.T("✓ Добавлен узел: %s", "✓ Node added: %s"), profile.Name))
	t.modal = ModalNone
}

func (t *TUI) submitSubURL() {
	url := strings.TrimSpace(t.modalInput)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		t.modalError = i18n.T("URL должен начинаться с https://", "URL must start with https://")
		return
	}
	t.cfg.SubscriptionURL = url
	grp := t.cfg.GetActiveGroup()
	if grp != nil {
		grp.URL = url
	}
	_ = config.SaveClientConfig(t.cfgPath, t.cfg)
	t.modal = ModalNone
	if grp != nil {
		t.fetchGroupSubscription(grp.ID)
	}
}

func (t *TUI) fetchGroupSubscription(groupID string) {
	if t.isUpdatingSub {
		return
	}
	t.isUpdatingSub = true
	go func() {
		count, err := config.FetchAndApplyGroupSubscription(t.cfg, groupID)
		t.mu.Lock()
		t.isUpdatingSub = false
		if err != nil {
			t.toast(i18n.T("Ошибка подписки: ", "Subscription error: ") + err.Error())
		} else {
			_ = config.SaveClientConfig(t.cfgPath, t.cfg)
			grp := t.cfg.GetActiveGroup()
			grpName := "Подписка"
			if grp != nil {
				grpName = grp.Name
			}
			t.selectedIdx = 0
			t.scrollOffset = 0
			t.toast(fmt.Sprintf(i18n.T("✓ Обновлено %d узлов в «%s»!", "✓ Updated %d nodes in «%s»!"), count, grpName))
		}
		t.mu.Unlock()
	}()
}

func (t *TUI) fetchSubscription() {
	grp := t.cfg.GetActiveGroup()
	if grp != nil {
		t.fetchGroupSubscription(grp.ID)
	}
}

func (t *TUI) toggleConnection() {
	if t.stats.Connected {
		_ = t.client.Stop()
		t.toast(i18n.T("Туннель остановлен", "Tunnel stopped"))
	} else {
		if err := t.client.Start(); err != nil {
			t.toast(i18n.T("Ошибка подключения: ", "Connection error: ") + err.Error())
		} else {
			t.toast(i18n.T("✓ Защищенный туннель запущен!", "✓ Secure tunnel started!"))
		}
	}
}

func (t *TUI) selectOrConnectActive() {
	servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
	if len(servers) == 0 || t.selectedIdx < 0 || t.selectedIdx >= len(servers) {
		return
	}

	sel := servers[t.selectedIdx]
	isAlreadyActive := (sel.ID == t.cfg.ActiveID)

	t.cfg.ActiveID = sel.ID
	t.client.UpdateProfile(sel)
	_ = config.SaveClientConfig(t.cfgPath, t.cfg)

	if !t.stats.Connected {
		if err := t.client.Start(); err != nil {
			t.toast(i18n.T("Ошибка подключения: ", "Connection error: ") + err.Error())
		} else {
			t.toast(fmt.Sprintf(i18n.T("✓ Подключено к: %s", "✓ Connected to: %s"), sel.Name))
		}
	} else {
		if isAlreadyActive {
			t.toast(fmt.Sprintf(i18n.T("Уже подключен к: %s", "Already connected to: %s"), sel.Name))
		} else {
			t.toast(fmt.Sprintf(i18n.T("✓ Переключено на: %s", "✓ Switched to: %s"), sel.Name))
		}
	}
}

func (t *TUI) toggleSysProxy() {
	if t.sysProxyStatus {
		_ = sysproxy.ClearSystemProxy()
		t.sysProxyStatus = false
		t.toast(i18n.T("Системный прокси отключен", "System proxy disabled"))
	} else {
		if err := sysproxy.SetSystemProxy(t.cfg.HttpAddr); err != nil {
			t.toast(i18n.T("Ошибка прокси: ", "Proxy error: ") + err.Error())
		} else {
			t.sysProxyStatus = true
			t.toast(i18n.T("✓ Системный прокси включен", "✓ System proxy enabled"))
		}
	}
}

func (t *TUI) pingAllServers() {
	if len(t.cfg.Servers) == 0 || t.isPingingAll {
		return
	}
	t.isPingingAll = true
	go func() {
		pings := config.PingAllConcurrently(t.cfg.Servers, 3*time.Second)
		t.mu.Lock()
		for i := range t.cfg.Servers {
			if p, ok := pings[t.cfg.Servers[i].ServerAddr]; ok {
				t.cfg.Servers[i].Ping = p
			}
		}
		t.isPingingAll = false
		_ = config.SaveClientConfig(t.cfgPath, t.cfg)
		t.toast(i18n.T("✓ Пинг всех узлов обновлен", "✓ All nodes ping updated"))
		t.mu.Unlock()
	}()
}

func (t *TUI) selectFastestServer() {
	servers := t.cfg.GetServersInGroup(t.cfg.ActiveGroupID)
	if len(servers) == 0 {
		servers = t.cfg.Servers
	}
	var bestIdx = -1
	var minPing = 999999

	for i, s := range servers {
		if s.Ping > 0 && s.Ping < minPing {
			minPing = s.Ping
			bestIdx = i
		}
	}

	if bestIdx == -1 {
		t.toast(i18n.T("Сначала нажмите [P] для проверки пинга!", "Press [P] first to measure pings!"))
		return
	}

	best := servers[bestIdx]
	t.selectedIdx = bestIdx
	t.cfg.ActiveID = best.ID
	t.client.UpdateProfile(best)
	_ = config.SaveClientConfig(t.cfgPath, t.cfg)

	if !t.stats.Connected {
		_ = t.client.Start()
	}
	t.toast(fmt.Sprintf(i18n.T("✓ Выбран самый быстрый: %s (%d ms)", "✓ Selected fastest: %s (%d ms)"), best.Name, best.Ping))
}

func (t *TUI) toast(msg string) {
	t.toastMsg = msg
	t.toastTimer = 12 // ~3 seconds at 250ms ticks
}

func (t *TUI) cleanupAndExit() {
	_ = t.client.Stop()
	RestoreTerminal()
	fmt.Println("\n⭐ ShadowStar TUI exited cleanly.")
	os.Exit(0)
}
