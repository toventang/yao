package dingtalk

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/attachment"
)

const defaultUploader = "__yao.attachment"

// FileResult holds attachment wrapper and metadata.
type FileResult struct {
	Wrapper  string
	MimeType string
	FileName string
}

// DownloadAndStoreURL downloads a file from URL and stores it through the
// attachment manager. Uses the URL as fingerprint for dedup.
func DownloadAndStoreURL(ctx context.Context, url, mimeType, fileName string, groups []string) (*FileResult, error) {
	manager, exists := attachment.Managers[defaultUploader]
	if !exists {
		return nil, fmt.Errorf("attachment manager %s not found", defaultUploader)
	}

	fingerprint := url
	probeID := fingerprintKey(fingerprint, groups)
	if manager.Exists(ctx, probeID) {
		wrapper := fmt.Sprintf("%s://%s", defaultUploader, probeID)
		return &FileResult{Wrapper: wrapper, MimeType: mimeType, FileName: fileName}, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if mimeType == "" {
		mimeType = resp.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
	}
	if fileName == "" {
		fileName = "file"
	}

	header := &attachment.FileHeader{
		FileHeader: &multipart.FileHeader{
			Filename: fileName,
			Size:     int64(len(data)),
			Header:   make(textproto.MIMEHeader),
		},
	}
	header.Header.Set("Content-Type", mimeType)
	header.Header.Set("Content-Fingerprint", fingerprint)

	option := attachment.UploadOption{
		OriginalFilename: fileName,
		Groups:           groups,
	}

	uploaded, err := manager.Upload(ctx, header, bytes.NewReader(data), option)
	if err != nil {
		return nil, fmt.Errorf("attachment upload: %w", err)
	}

	wrapper := fmt.Sprintf("%s://%s", defaultUploader, uploaded.ID)
	return &FileResult{Wrapper: wrapper, MimeType: mimeType, FileName: fileName}, nil
}

// ResolveMedia downloads and stores all media items in a ConvertedMessage.
func ResolveMedia(ctx context.Context, cm *ConvertedMessage, groups []string) {
	if cm == nil {
		return
	}
	for i := range cm.MediaItems {
		mi := &cm.MediaItems[i]
		if mi.URL == "" {
			continue
		}
		result, err := DownloadAndStoreURL(ctx, mi.URL, mi.MimeType, mi.FileName, groups)
		if err != nil {
			log.Error("dingtalk ResolveMedia: %s %s: %v", mi.Type, mi.URL, err)
			continue
		}
		mi.Wrapper = result.Wrapper
		if result.MimeType != "" {
			mi.MimeType = result.MimeType
		}
	}
}

func fingerprintKey(key string, groups []string) string {
	parts := make([]string, 0, len(groups)+1)
	parts = append(parts, groups...)
	parts = append(parts, key)
	storagePath := strings.Join(parts, "/")
	hash := md5.Sum([]byte(storagePath))
	return hex.EncodeToString(hash[:])
}
