package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is persisted at ~/.opendrop/config.yaml
type Config struct {
	DeviceID          string   `yaml:"device_id"`
	DeviceName        string   `yaml:"device_name"`
	Port              int      `yaml:"port"`
	PSKToken          string   `yaml:"psk_token"`
	DownloadDir       string   `yaml:"download_dir"`
	ClipboardEnabled  bool     `yaml:"clipboard_enabled"`
	ClipboardPaused   bool     `yaml:"clipboard_paused"`
	ManualPeers       []string `yaml:"peers"` // e.g. ["192.168.1.50:53317"]
	MDNSEnabled       bool     `yaml:"mdns_enabled"`
	ClipboardMaxBytes int      `yaml:"clipboard_max_bytes"`
}

func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".opendrop"
	}
	return filepath.Join(home, ".opendrop")
}

func Path() string {
	return filepath.Join(Dir(), "config.yaml")
}

func DefaultDownloadDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "downloads"
	}
	// Windows uses Downloads too; keep same layout
	return filepath.Join(home, "Downloads", "OpenDrop")
}

func defaultDeviceName() string {
	h, err := os.Hostname()
	if err == nil && h != "" {
		return h
	}
	return "opendrop"
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func Defaults() *Config {
	return &Config{
		DeviceID:          "dev-" + randomHex(8),
		DeviceName:        defaultDeviceName(),
		Port:              53317,
		PSKToken:          randomHex(32),
		DownloadDir:       DefaultDownloadDir(),
		ClipboardEnabled:  true,
		ClipboardPaused:   false,
		ManualPeers:       []string{},
		MDNSEnabled:       true,
		ClipboardMaxBytes: 1 << 20, // 1 MiB text
	}
}

func Load() (*Config, error) {
	p := Path()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			c := Defaults()
			if err := c.Save(); err != nil {
				return nil, err
			}
			fmt.Printf("Created new config at %s\nYour pairing token: %s\nCopy it to your other device with: opendrop pair --token %s\n", p, c.PSKToken, c.PSKToken)
			return c, nil
		}
		return nil, err
	}
	c := Defaults()
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if c.Port == 0 {
		c.Port = 53317
	}
	if c.DownloadDir == "" {
		c.DownloadDir = DefaultDownloadDir()
	}
	return c, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(c.DownloadDir, 0o755); err != nil {
		// non-fatal: download dir may be created later
		fmt.Printf("warning: cannot create download dir: %v\n", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0o600)
}
