package updater

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	CurrentVersion = "v2.0.7"
	DefaultRepo    = "komendant-zero/shadowstar"
)

// ReleaseInfo holds GitHub release metadata
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	IsNewer     bool   `json:"is_newer"`
	DownloadURL string `json:"download_url"`
	APKURL      string `json:"apk_url,omitempty"`
}

type ghReleaseResponse struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckUpdate queries GitHub Releases for the latest version
func CheckUpdate(repo string) (*ReleaseInfo, error) {
	if repo == "" {
		repo = DefaultRepo
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ShadowStar-Client/"+CurrentVersion)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var ghResp ghReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&ghResp); err != nil {
		return nil, err
	}

	latestTag := strings.TrimSpace(ghResp.TagName)
	isNewer := compareVersions(latestTag, CurrentVersion) > 0

	// Find best asset for current OS/Arch
	downloadURL := ""
	apkURL := ""
	for _, a := range ghResp.Assets {
		name := strings.ToLower(a.Name)
		if strings.HasSuffix(name, ".apk") {
			apkURL = a.BrowserDownloadURL
		}
		if runtime.GOOS == "windows" {
			if strings.HasSuffix(name, "-windows-amd64.zip") || (strings.Contains(name, "windows") && strings.HasSuffix(name, ".zip")) {
				downloadURL = a.BrowserDownloadURL
			} else if strings.HasSuffix(name, ".exe") && !strings.Contains(name, "server") && !strings.Contains(name, "mobile") {
				if downloadURL == "" {
					downloadURL = a.BrowserDownloadURL
				}
			}
		} else if runtime.GOOS == "linux" {
			if strings.Contains(name, "linux") && strings.Contains(name, runtime.GOARCH) && strings.HasSuffix(name, ".tar.gz") {
				downloadURL = a.BrowserDownloadURL
			}
		}
	}
	if downloadURL == "" {
		if apkURL != "" {
			downloadURL = apkURL
		} else {
			downloadURL = ghResp.HTMLURL
		}
	}

	return &ReleaseInfo{
		TagName:     latestTag,
		Name:        ghResp.Name,
		Body:        ghResp.Body,
		HTMLURL:     ghResp.HTMLURL,
		PublishedAt: ghResp.PublishedAt.Format("02.01.2006"),
		IsNewer:     isNewer,
		DownloadURL: downloadURL,
		APKURL:      apkURL,
	}, nil
}

// ProgressCallback reports progress from 0 to 100 with a status string
type ProgressCallback func(percent int, status string)

// DownloadAndApplyUpdate downloads the latest Windows release asset from GitHub,
// extracts shadowstar.exe, safely replaces the running binary with process-wait verification,
// and restarts the application in an independent console window.
func DownloadAndApplyUpdate(directURL string, cb ProgressCallback, beforeRestart ...func()) error {
	if cb == nil {
		cb = func(int, string) {}
	}

	cb(5, "Поиск обновления на GitHub...")

	downloadURL := directURL
	if downloadURL == "" || !strings.Contains(downloadURL, "/download/") {
		rel, err := CheckUpdate(DefaultRepo)
		if err != nil {
			return fmt.Errorf("ошибка проверки релиза: %w", err)
		}
		downloadURL = rel.DownloadURL
	}

	if downloadURL == "" || !strings.Contains(downloadURL, "/download/") {
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", DefaultRepo)
		req, err := http.NewRequest("GET", apiURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", "ShadowStar-Client/"+CurrentVersion)
			client := &http.Client{Timeout: 10 * time.Second}
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				var ghResp ghReleaseResponse
				if json.NewDecoder(resp.Body).Decode(&ghResp) == nil {
					for _, a := range ghResp.Assets {
						name := strings.ToLower(a.Name)
						if strings.HasSuffix(name, "-windows-amd64.zip") || strings.HasSuffix(name, ".zip") {
							downloadURL = a.BrowserDownloadURL
							break
						}
					}
				}
			}
		}
	}

	if downloadURL == "" || strings.HasSuffix(downloadURL, "/releases/latest") {
		return fmt.Errorf("не удалось найти установочный архив для Windows в релизах GitHub")
	}

	cb(15, "Подключение к серверу загрузки...")

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("User-Agent", "ShadowStar-Client/"+CurrentVersion)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка загрузки с GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("сервер вернул статус: %d %s", resp.StatusCode, resp.Status)
	}

	contentLength := resp.ContentLength
	var downloaded int64

	var buf bytes.Buffer
	chunk := make([]byte, 32*1024)
	lastReport := time.Now()

	for {
		n, rerr := resp.Body.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
			downloaded += int64(n)

			if time.Since(lastReport) > 100*time.Millisecond || rerr != nil {
				lastReport = time.Now()
				pct := 20
				if contentLength > 0 {
					pct = 20 + int((float64(downloaded)/float64(contentLength))*60.0)
					if pct > 80 {
						pct = 80
					}
					cb(pct, fmt.Sprintf("Скачивание: %d%% (%.1f / %.1f МБ)",
						pct, float64(downloaded)/(1024*1024), float64(contentLength)/(1024*1024)))
				} else {
					cb(pct, fmt.Sprintf("Скачивание: %.1f МБ...", float64(downloaded)/(1024*1024)))
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("ошибка скачивания файла: %w", rerr)
		}
	}

	cb(85, "Распаковка исполняемого файла...")

	var exeBytes []byte
	payloadBytes := buf.Bytes()

	// Check if downloaded file is a ZIP archive
	if len(payloadBytes) > 4 && payloadBytes[0] == 0x50 && payloadBytes[1] == 0x4B {
		zr, err := zip.NewReader(bytes.NewReader(payloadBytes), int64(len(payloadBytes)))
		if err != nil {
			return fmt.Errorf("ошибка чтения zip-архива: %w", err)
		}
		for _, f := range zr.File {
			if strings.EqualFold(filepath.Base(f.Name), "shadowstar.exe") {
				rc, err := f.Open()
				if err != nil {
					return fmt.Errorf("ошибка извлечения shadowstar.exe: %w", err)
				}
				exeBytes, err = io.ReadAll(rc)
				_ = rc.Close()
				if err != nil {
					return fmt.Errorf("ошибка чтения shadowstar.exe: %w", err)
				}
				break
			}
		}
		if len(exeBytes) == 0 {
			return fmt.Errorf("в архиве обновления не найден файл shadowstar.exe")
		}
	} else if len(payloadBytes) > 2 && payloadBytes[0] == 'M' && payloadBytes[1] == 'Z' {
		// Direct EXE binary
		exeBytes = payloadBytes
	} else {
		return fmt.Errorf("некорректный формат полученного файла обновления")
	}

	cb(92, "Установка новой версии...")

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к программе: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	newExePath := filepath.Join(exeDir, "shadowstar.exe.new")

	_ = os.Remove(newExePath)
	if err := os.WriteFile(newExePath, exeBytes, 0755); err != nil {
		return fmt.Errorf("ошибка сохранения нового файла: %w", err)
	}

	cb(98, "Перезапуск ShadowStar...")

	// Run clean shutdown callbacks (e.g. restore terminal VT100 mode, disable sysproxy, stop TUN/Client)
	for _, fn := range beforeRestart {
		if fn != nil {
			fn()
		}
	}

	cleanArgs := cleanRestartArgs()

	if runtime.GOOS == "windows" {
		batPath := filepath.Join(exeDir, "shadowstar_updater.bat")
		batContent := fmt.Sprintf(`@echo off
setlocal enabledelayedexpansion

set "OLD_PID=%%1"
set "NEW_EXE=%%~2"
set "TARGET_EXE=%%~3"
shift
shift
shift

set "APP_ARGS="
:args_loop
if "%%~1"=="" goto args_done
set "APP_ARGS=!APP_ARGS! "%%~1""
shift
goto args_loop
:args_done

:: 1. Wait for current instance to completely terminate and release all port bindings
if not "!OLD_PID!"=="" (
    set /a WAIT_CNT=0
    :wait_pid
    tasklist /fi "PID eq !OLD_PID!" 2>nul | findstr /i "!OLD_PID!" >nul
    if not errorlevel 1 (
        set /a WAIT_CNT+=1
        if !WAIT_CNT! leq 40 (
            timeout /t 1 /nobreak >nul 2>nul || ping 127.0.0.1 -n 2 >nul
            goto wait_pid
        )
    )
)

:: Extra brief settling delay
ping 127.0.0.1 -n 2 >nul

:: 2. Safely replace binary with retry loop (handles AV scanners / file indexing)
set /a RETRY=0
:replace_loop
if exist "!NEW_EXE!" (
    if exist "!TARGET_EXE!" (
        del /f /q "!TARGET_EXE!.old" >nul 2>nul
        move /y "!TARGET_EXE!" "!TARGET_EXE!.old" >nul 2>nul
    )
    move /y "!NEW_EXE!" "!TARGET_EXE!" >nul 2>nul
    if errorlevel 1 (
        set /a RETRY+=1
        if !RETRY! leq 30 (
            timeout /t 1 /nobreak >nul 2>nul || ping 127.0.0.1 -n 2 >nul
            goto replace_loop
        )
    )
)

:: 3. Clean up backup
del /f /q "!TARGET_EXE!.old" >nul 2>nul

:: 4. Start updated application in an independent console window
if exist "!TARGET_EXE!" (
    start "" "!TARGET_EXE!" !APP_ARGS!
)

:: 5. Self-delete updater batch script
del /f /q "%%~f0" >nul 2>nul
`)
		if err := os.WriteFile(batPath, []byte(batContent), 0755); err != nil {
			return fmt.Errorf("ошибка создания скрипта перезапуска: %w", err)
		}

		cmdArgs := []string{"/c", batPath, fmt.Sprintf("%d", os.Getpid()), newExePath, exePath}
		cmdArgs = append(cmdArgs, cleanArgs...)
		cmd := exec.Command("cmd.exe", cmdArgs...)
		cmd.Dir = exeDir
		// Windows specific: launch in new console & detached so parent exit doesn't kill it
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: 0x00000010 | 0x00000008, // CREATE_NEW_CONSOLE | DETACHED_PROCESS
		}
		if err := cmd.Start(); err != nil {
			cmd.SysProcAttr = nil
			_ = cmd.Start()
		}

		os.Exit(0)
		return nil
	}

	// Unix (Linux / macOS) fallback
	shPath := filepath.Join(exeDir, "shadowstar_updater.sh")
	shContent := fmt.Sprintf(`#!/bin/sh
OLD_PID="$1"
NEW_EXE="$2"
TARGET_EXE="$3"
shift 3

if [ -n "$OLD_PID" ]; then
    while kill -0 "$OLD_PID" 2>/dev/null; do
        sleep 0.3
    done
fi

mv -f "$NEW_EXE" "$TARGET_EXE"
chmod +x "$TARGET_EXE"
exec "$TARGET_EXE" "$@" &
rm -f "$0"
`)
	if err := os.WriteFile(shPath, []byte(shContent), 0755); err == nil {
		cmdArgs := []string{shPath, fmt.Sprintf("%d", os.Getpid()), newExePath, exePath}
		cmdArgs = append(cmdArgs, cleanArgs...)
		cmd := exec.Command("sh", cmdArgs...)
		cmd.Dir = exeDir
		_ = cmd.Start()
		os.Exit(0)
		return nil
	}

	// Direct rename fallback
	_ = os.Rename(newExePath, exePath)
	cmd := exec.Command(exePath, cleanArgs...)
	cmd.Dir = exeDir
	_ = cmd.Start()
	os.Exit(0)
	return nil
}

func cleanRestartArgs() []string {
	var clean []string
	for _, arg := range os.Args[1:] {
		if arg == "-update" || arg == "--update" {
			continue
		}
		clean = append(clean, arg)
	}
	return clean
}

// CleanupOldFiles removes leftover .old or updater scripts from previous update
func CleanupOldFiles() {
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		_ = os.Remove(exe + ".old")
		_ = os.Remove(exe + ".new")
		_ = os.Remove(filepath.Join(exeDir, "shadowstar.exe.old"))
		_ = os.Remove(filepath.Join(exeDir, "shadowstar.exe.new"))
		_ = os.Remove(filepath.Join(exeDir, "shadowstar_update.bat"))
		_ = os.Remove(filepath.Join(exeDir, "shadowstar_updater.bat"))
		_ = os.Remove(filepath.Join(exeDir, "shadowstar_updater.sh"))
	}
}

// OpenURL opens the specified URL in the system's default web browser securely
func OpenURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("insecure or invalid URL scheme: %s", rawURL)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

// compareVersions returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	clean1 := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	clean2 := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	parts1 := strings.Split(clean1, ".")
	parts2 := strings.Split(clean2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		num1 := 0
		num2 := 0
		if i < len(parts1) {
			_, _ = fmt.Sscanf(parts1[i], "%d", &num1)
		}
		if i < len(parts2) {
			_, _ = fmt.Sscanf(parts2[i], "%d", &num2)
		}
		if num1 > num2 {
			return 1
		}
		if num1 < num2 {
			return -1
		}
	}
	return 0
}
