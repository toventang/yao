package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	apiBase   = "https://api.dingtalk.com"
	oauthBase = "https://api.dingtalk.com/v1.0/oauth2/accessToken"
)

// Bot represents a DingTalk bot instance.
type Bot struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client

	accessToken  string
	tokenExpires time.Time
}

// NewBot creates a Bot bound to DingTalk app credentials.
func NewBot(clientID, clientSecret string) *Bot {
	return &Bot{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// ClientID returns the client ID.
func (b *Bot) ClientID() string { return b.clientID }

// ClientSecret returns the client secret.
func (b *Bot) ClientSecret() string { return b.clientSecret }

// GetAccessToken returns a valid access token, refreshing if necessary.
func (b *Bot) GetAccessToken(ctx context.Context) (string, error) {
	if b.accessToken != "" && time.Now().Before(b.tokenExpires) {
		return b.accessToken, nil
	}

	body, _ := json.Marshal(map[string]string{
		"appKey":    b.clientID,
		"appSecret": b.clientSecret,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", oauthBase, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("dingtalk get token: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal token: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("dingtalk token empty, body=%s", string(respBody))
	}

	b.accessToken = result.AccessToken
	b.tokenExpires = time.Now().Add(time.Duration(result.ExpireIn-60) * time.Second)
	return b.accessToken, nil
}

// GetBotInfo verifies the bot credentials by fetching the access token.
func (b *Bot) GetBotInfo(ctx context.Context) error {
	_, err := b.GetAccessToken(ctx)
	return err
}

// apiRequest makes an authenticated API call to DingTalk.
func (b *Bot) apiRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	token, err := b.GetAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	url := apiBase + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dingtalk api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read api response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dingtalk api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
