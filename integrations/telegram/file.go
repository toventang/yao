package telegram

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/attachment"
)

const defaultUploader = "__yao.attachment"

// FileResult holds the attachment wrapper and metadata for a downloaded
// Telegram file that has been stored via the attachment manager.
type FileResult struct {
	Wrapper  string // e.g. __yao.attachment://ccd472d11feb96e03a3fc468f494045c
	MimeType string
	FileName string
}

// GetFile retrieves file metadata (including download path) for a given file_id.
// Uses raw HTTP for compatibility with both official and local Bot API servers.
func (b *Bot) GetFile(ctx context.Context, fileID string) (*models.File, error) {
	body, err := json.Marshal(map[string]string{"file_id": fileID})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", b.botURL()+"/getFile", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		OK     bool        `json:"ok"`
		Result models.File `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("getFile: API returned ok=false, body=%s", string(respBody))
	}
	return &result.Result, nil
}

// DownloadFile downloads a file given its file_path from GetFile.
// Returns the response body (caller must close), content type, and file size.
// When file_path is an absolute path (local Bot API server --local mode),
// the file is read directly from disk instead of HTTP download.
func (b *Bot) DownloadFile(ctx context.Context, filePath string) (io.ReadCloser, string, int64, error) {
	if strings.HasPrefix(filePath, "/") {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, "", 0, fmt.Errorf("open local file: %w", err)
		}
		info, _ := f.Stat()
		var size int64
		if info != nil {
			size = info.Size()
		}
		return f, "application/octet-stream", size, nil
	}

	url := b.fileURL(filePath)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", 0, err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", 0, fmt.Errorf("download file: status %d", resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("Content-Type"), resp.ContentLength, nil
}

// DownloadAndStore downloads a Telegram file by file_id, stores it through the
// attachment manager, and returns the wrapper string. Uses file_unique_id as
// the Content-Fingerprint so that the same Telegram file is never downloaded
// and stored twice — attachment's built-in fingerprint dedup handles it.
func (b *Bot) DownloadAndStore(ctx context.Context, tgFileID, fileUniqueID, mimeType, filename string, groups []string) (*FileResult, error) {

	manager, exists := attachment.Managers[defaultUploader]
	if !exists {
		return nil, fmt.Errorf("attachment manager %s not found", defaultUploader)
	}

	probeID := fingerprintFileID(fileUniqueID, groups)
	if manager.Exists(ctx, probeID) {
		wrapper := fmt.Sprintf("%s://%s", defaultUploader, probeID)
		log.Trace("telegram file: cache hit file_unique_id=%s wrapper=%s", fileUniqueID, wrapper)
		return &FileResult{Wrapper: wrapper, MimeType: mimeType, FileName: filename}, nil
	}

	fileMeta, err := b.GetFile(ctx, tgFileID)
	if err != nil {
		return nil, fmt.Errorf("getFile: %w", err)
	}

	body, contentType, size, err := b.DownloadFile(ctx, fileMeta.FilePath)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer body.Close()

	if contentType != "" && mimeType == "application/octet-stream" {
		mimeType = contentType
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if size <= 0 {
		size = int64(len(data))
	}

	ext := filepath.Ext(filename)

	header := &attachment.FileHeader{
		FileHeader: &multipart.FileHeader{
			Filename: filename,
			Size:     size,
			Header:   make(textproto.MIMEHeader),
		},
	}
	header.Header.Set("Content-Type", mimeType)
	header.Header.Set("Content-Fingerprint", fileUniqueID)
	if ext != "" {
		header.Header.Set("Content-Extension", ext)
	}

	option := attachment.UploadOption{
		OriginalFilename: filename,
		Groups:           groups,
	}

	uploaded, err := manager.Upload(ctx, header, bytes.NewReader(data), option)
	if err != nil {
		return nil, fmt.Errorf("attachment upload: %w", err)
	}

	wrapper := fmt.Sprintf("%s://%s", defaultUploader, uploaded.ID)
	return &FileResult{Wrapper: wrapper, MimeType: mimeType, FileName: filename}, nil
}

// ResolveMedia downloads and stores all media items in the ConvertedMessage,
// filling each MediaItem.Wrapper with the attachment wrapper string.
// Items that fail to download are logged and left with an empty Wrapper.
func (b *Bot) ResolveMedia(ctx context.Context, cm *ConvertedMessage, groups []string) {
	if cm == nil {
		return
	}
	for i := range cm.MediaItems {
		mi := &cm.MediaItems[i]
		result, err := b.DownloadAndStore(ctx, mi.FileID, mi.FileUniqueID, mi.MimeType, mi.FileName, groups)
		if err != nil {
			log.Error("telegram ResolveMedia: %s %s: %v", mi.Type, mi.FileID, err)
			continue
		}
		mi.Wrapper = result.Wrapper
		if result.MimeType != "" {
			mi.MimeType = result.MimeType
		}
	}
}

// fingerprintFileID reproduces the file_id that attachment.Manager would
// generate when Content-Fingerprint is set, so we can probe Exists() before
// downloading anything.
func fingerprintFileID(fileUniqueID string, groups []string) string {
	parts := make([]string, 0, len(groups)+1)
	parts = append(parts, groups...)
	parts = append(parts, fileUniqueID)
	storagePath := strings.Join(parts, "/")
	hash := md5.Sum([]byte(storagePath))
	return hex.EncodeToString(hash[:])
}
