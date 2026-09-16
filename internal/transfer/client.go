package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// SendFile POSTs a local file to http://addr/api/v1/files with PSK auth.
// Streams from disk; computes sha256 first for integrity header.
func SendFile(addr, psk, sender, filePath string) (remotePath string, nBytes int64, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", 0, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}

	var buf bytes.Buffer
	// NOTE: for very large files this buffers in RAM via bytes.Buffer.
	// MVP keeps it simple; chunked/resumable upload is the V1.1 upgrade.
	// For now cap at 512 MiB to avoid OOM.
	if st.Size() > 512<<20 {
		return "", 0, fmt.Errorf("file too large for MVP (>512 MiB): %s", filePath)
	}
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("sender", sender)
	part, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", 0, err
	}
	n, err := io.Copy(part, f)
	if err != nil {
		return "", 0, err
	}
	_ = mw.Close()

	url := fmt.Sprintf("http://%s/api/v1/files", addr)
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+psk)
	req.Header.Set("X-Sha256", sum)
	req.ContentLength = int64(buf.Len())

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("server %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(body, &out)
	return out.Path, n, nil
}

// PushClipboard POSTs text to a peer's /api/v1/clipboard.
func PushClipboard(addr, psk string, msg ClipboardMessage) error {
	data, _ := json.Marshal(msg)
	url := fmt.Sprintf("http://%s/api/v1/clipboard", addr)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+psk)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clipboard push %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// ProbeHealth checks http://addr/api/v1/health and returns device name.
func ProbeHealth(addr string, timeout time.Duration) (name, id string, ok bool) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(fmt.Sprintf("http://%s/api/v1/health", addr))
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	var out struct {
		DeviceName string `json:"device_name"`
		DeviceID   string `json:"device_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", false
	}
	return out.DeviceName, out.DeviceID, true
}
