package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shadowstar/shadowstar/pkg/client"
)

func TestPersistentConfigPath(t *testing.T) {
	p := GetPersistentConfigPath()
	if p == "" {
		t.Fatalf("expected non-empty persistent config path")
	}
	t.Logf("Persistent config path: %s", p)

	mp := GetPersistentMobileConfigPath()
	if mp == "" || mp == p {
		t.Fatalf("expected valid distinct mobile persistent config path, got %s", mp)
	}
	t.Logf("Persistent mobile config path: %s", mp)
}

func TestPersistentRestoreAfterReinstall(t *testing.T) {
	tempDir := t.TempDir()
	persistentDir := filepath.Join(tempDir, "user_persistent_storage")
	t.Setenv("SHADOWSTAR_CONFIG_DIR", persistentDir)

	// 1. Initial simulated installation in folder A
	folderA := filepath.Join(tempDir, "install_folder_a")
	_ = os.MkdirAll(folderA, 0755)
	cfgPathA := filepath.Join(folderA, "shadowstar-client.json")

	initialCfg := &client.Config{
		SocksAddr: "127.0.0.1:10808",
		HttpAddr:  "127.0.0.1:10809",
		ActiveID:  "my-custom-vps",
		Servers: []client.ServerProfile{
			{
				ID:            "my-custom-vps",
				Name:          "Personal Fast VPS",
				ServerAddr:    "198.51.100.55:4433",
				MasterKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				SNI:           "www.microsoft.com",
				EnableChunker: true,
			},
		},
	}

	// Save in folder A (this also mirrors to persistent storage)
	if err := SaveClientConfig(cfgPathA, initialCfg); err != nil {
		t.Fatalf("failed to save config in folder A: %v", err)
	}

	// 2. Simulate complete uninstallation: folder A is completely deleted
	_ = os.RemoveAll(folderA)

	// 3. Simulate reinstallation in completely new folder B
	folderB := filepath.Join(tempDir, "fresh_reinstall_folder_b")
	_ = os.MkdirAll(folderB, 0755)
	cfgPathB := filepath.Join(folderB, "shadowstar-client.json")

	// Load config in folder B where no file exists yet
	restoredCfg, err := LoadClientConfig(cfgPathB)
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}

	// Verify that our server from folder A was automatically restored into folder B!
	if len(restoredCfg.Servers) == 0 {
		t.Fatalf("expected servers to be restored after reinstall, but got 0 servers")
	}

	found := false
	for _, s := range restoredCfg.Servers {
		if s.ServerAddr == "198.51.100.55:4433" && s.Name == "Personal Fast VPS" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("restored config does not contain the original server from folder A")
	}
	t.Logf("✓ Successfully restored %d servers into fresh install directory from persistent storage", len(restoredCfg.Servers))
}
