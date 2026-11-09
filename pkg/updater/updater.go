package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	CurrentVersion = "v1.3.0"
	DefaultRepo    = "shadowstar/shadowstar"
)

// ReleaseInfo holds GitHub release metadata
type ReleaseInfo struct {
	TagName      string `json:"tag_name"`
	Name         string `json:"name"`
	Body         string `json:"body"`
	HTMLURL      string `json:"html_url"`
	PublishedAt  string `json:"published_at"`
	IsNewer      bool   `json:"is_newer"`
	DownloadURL  string `json:"download_url"`
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
	downloadURL := ghResp.HTMLURL
	for _, a := range ghResp.Assets {
		name := strings.ToLower(a.Name)
		if runtime.GOOS == "windows" && (strings.Contains(name, "windows") || strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".zip")) {
			downloadURL = a.BrowserDownloadURL
			break
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
	}, nil
}

// OpenURL opens the specified URL in the system's default web browser
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
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
