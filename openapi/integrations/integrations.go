package integrations

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/event"
	"github.com/yaoapp/yao/openapi/response"
)

// WebhookPayload is the event payload pushed to "integration.webhook.{provider}".
// Subscribers receive this and handle it according to their own protocol.
type WebhookPayload struct {
	Provider string            `json:"provider"`
	AppID    string            `json:"app_id"`
	Method   string            `json:"method"`
	Body     []byte            `json:"body,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Query    map[string]string `json:"query,omitempty"`
}

// Attach registers the integrations webhook endpoints.
// These are public endpoints (no OAuth) since external platforms push here.
func Attach(group *gin.RouterGroup) {
	group.GET("/:provider/:app_id", webhookHandler)
	group.POST("/:provider/:app_id", webhookHandler)
}

// webhookHandler receives webhooks from external platforms, packs the raw
// request into a WebhookPayload, and pushes an event for async processing.
// It returns HTTP 200 immediately — subscribers handle the rest.
func webhookHandler(c *gin.Context) {
	provider := c.Param("provider")
	appID := c.Param("app_id")

	if provider == "" || appID == "" {
		response.RespondWithError(c, response.StatusBadRequest, &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "provider and app_id are required",
		})
		return
	}

	payload := WebhookPayload{
		Provider: provider,
		AppID:    appID,
		Method:   c.Request.Method,
	}

	// Read body for POST/PUT/PATCH
	if c.Request.Body != nil && c.Request.Method != http.MethodGet {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Error("integrations webhook: read body failed provider=%s app_id=%s: %v", provider, appID, err)
			c.Status(http.StatusOK)
			return
		}
		payload.Body = body
	}

	payload.Headers = flattenHeaders(c.Request.Header)
	payload.Query = flattenQuery(c.Request.URL.Query())

	c.Status(http.StatusOK)

	eventType := "integration.webhook." + provider
	if _, err := event.Push(context.Background(), eventType, payload); err != nil {
		log.Error("integrations webhook: event.Push failed type=%s app_id=%s: %v", eventType, appID, err)
	}
}

func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

func flattenQuery(q map[string][]string) map[string]string {
	if len(q) == 0 {
		return nil
	}
	out := make(map[string]string, len(q))
	for k, v := range q {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}
