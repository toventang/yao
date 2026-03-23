package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SendTextMessage sends a text message to a conversation using the session webhook.
func SendTextMessage(ctx context.Context, sessionWebhook, text string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": text,
		},
	})
	return postWebhook(ctx, sessionWebhook, body)
}

// SendMarkdownMessage sends a markdown message via session webhook.
func SendMarkdownMessage(ctx context.Context, sessionWebhook, title, text string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  text,
		},
	})
	return postWebhook(ctx, sessionWebhook, body)
}

// SendImageMessage sends an image via session webhook using media_id.
func SendImageMessage(ctx context.Context, sessionWebhook, mediaID string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"msgtype": "image",
		"image": map[string]string{
			"mediaId": mediaID,
		},
	})
	return postWebhook(ctx, sessionWebhook, body)
}

// SendFileMessage sends a file via session webhook using media_id.
func SendFileMessage(ctx context.Context, sessionWebhook, mediaID, fileName, fileType string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"msgtype": "file",
		"file": map[string]string{
			"mediaId":  mediaID,
			"fileName": fileName,
			"fileType": fileType,
		},
	})
	return postWebhook(ctx, sessionWebhook, body)
}

// ReplyText sends a text reply to a conversation using the Robot OpenAPI.
func (b *Bot) ReplyText(ctx context.Context, openConversationID, text string) error {
	token, err := b.GetAccessToken(ctx)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]interface{}{
		"robotCode":          b.clientID,
		"openConversationId": openConversationID,
		"msgKey":             "sampleText",
		"msgParam":           fmt.Sprintf(`{"content":"%s"}`, text),
	})

	req, err := http.NewRequestWithContext(ctx, "POST",
		apiBase+"/v1.0/robot/oToMessages/batchSend", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk reply: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dingtalk reply: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func postWebhook(ctx context.Context, webhookURL string, body []byte) error {
	if webhookURL == "" {
		return fmt.Errorf("empty session webhook URL")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk webhook post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dingtalk webhook: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}
