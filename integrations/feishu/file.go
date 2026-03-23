package feishu

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/attachment"
)

const defaultUploader = "__yao.attachment"

// FileResult holds the attachment wrapper and metadata for a stored file.
type FileResult struct {
	Wrapper  string
	MimeType string
	FileName string
}

// DownloadAndStoreImage downloads a Feishu image by image_key and stores it
// through the attachment manager. Uses image_key as the fingerprint for dedup.
func (b *Bot) DownloadAndStoreImage(ctx context.Context, messageID, imageKey string, groups []string) (*FileResult, error) {
	manager, exists := attachment.Managers[defaultUploader]
	if !exists {
		return nil, fmt.Errorf("attachment manager %s not found", defaultUploader)
	}

	probeID := fingerprintKey(imageKey, groups)
	if manager.Exists(ctx, probeID) {
		wrapper := fmt.Sprintf("%s://%s", defaultUploader, probeID)
		return &FileResult{Wrapper: wrapper, MimeType: "image/jpeg", FileName: imageKey + ".jpg"}, nil
	}

	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(imageKey).
		Type("image").
		Build()

	resp, err := b.client.Im.MessageResource.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("feishu get image resource: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("feishu get image resource: code=%d", resp.Code)
	}

	data, err := io.ReadAll(resp.File)
	if err != nil {
		return nil, fmt.Errorf("read image body: %w", err)
	}

	return storeData(ctx, manager, imageKey, "image/jpeg", imageKey+".jpg", data, groups)
}

// DownloadAndStoreFile downloads a Feishu file by file_key and stores it.
func (b *Bot) DownloadAndStoreFile(ctx context.Context, messageID, fileKey, mimeType, fileName string, groups []string) (*FileResult, error) {
	manager, exists := attachment.Managers[defaultUploader]
	if !exists {
		return nil, fmt.Errorf("attachment manager %s not found", defaultUploader)
	}

	probeID := fingerprintKey(fileKey, groups)
	if manager.Exists(ctx, probeID) {
		wrapper := fmt.Sprintf("%s://%s", defaultUploader, probeID)
		return &FileResult{Wrapper: wrapper, MimeType: mimeType, FileName: fileName}, nil
	}

	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(fileKey).
		Type("file").
		Build()

	resp, err := b.client.Im.MessageResource.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("feishu get file resource: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("feishu get file resource: code=%d", resp.Code)
	}

	data, err := io.ReadAll(resp.File)
	if err != nil {
		return nil, fmt.Errorf("read file body: %w", err)
	}

	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if fileName == "" {
		fileName = resp.FileName
		if fileName == "" {
			fileName = fileKey
		}
	}

	return storeData(ctx, manager, fileKey, mimeType, fileName, data, groups)
}

// ResolveMedia downloads and stores all media items in a ConvertedMessage.
func (b *Bot) ResolveMedia(ctx context.Context, cm *ConvertedMessage, groups []string) {
	if cm == nil {
		return
	}
	for i := range cm.MediaItems {
		mi := &cm.MediaItems[i]
		var result *FileResult
		var err error

		switch mi.Type {
		case MediaImage:
			result, err = b.DownloadAndStoreImage(ctx, cm.MessageID, mi.Key, groups)
		default:
			result, err = b.DownloadAndStoreFile(ctx, cm.MessageID, mi.Key, mi.MimeType, mi.FileName, groups)
		}

		if err != nil {
			log.Error("feishu ResolveMedia: %s %s: %v", mi.Type, mi.Key, err)
			continue
		}
		mi.Wrapper = result.Wrapper
		if result.MimeType != "" {
			mi.MimeType = result.MimeType
		}
	}
}

func storeData(ctx context.Context, manager *attachment.Manager, fingerprint, mimeType, fileName string, data []byte, groups []string) (*FileResult, error) {
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

func fingerprintKey(key string, groups []string) string {
	parts := make([]string, 0, len(groups)+1)
	parts = append(parts, groups...)
	parts = append(parts, key)
	storagePath := strings.Join(parts, "/")
	hash := md5.Sum([]byte(storagePath))
	return hex.EncodeToString(hash[:])
}
