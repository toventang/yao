package robot

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/yaoapp/yao/integrations/dingtalk"
	"github.com/yaoapp/yao/integrations/discord"
	"github.com/yaoapp/yao/integrations/feishu"
	"github.com/yaoapp/yao/integrations/telegram"
	"github.com/yaoapp/yao/openapi/response"
)

// VerifyIntegrationRequest — POST body for credential verification.
type VerifyIntegrationRequest struct {
	Provider string         `json:"provider" binding:"required"` // telegram | feishu | dingtalk | discord
	Config   map[string]any `json:"config" binding:"required"`
}

// VerifyIntegrationResponse — returned to the frontend.
type VerifyIntegrationResponse struct {
	Valid bool           `json:"valid"`
	Info  map[string]any `json:"info,omitempty"`
	Error string         `json:"error,omitempty"`
}

// VerifyIntegration tests whether the supplied credentials are valid
// by making a lightweight API call to the target platform.
//
// POST /v1/agent/robots/integrations/verify
func VerifyIntegration(c *gin.Context) {
	var req VerifyIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.RespondWithError(c, response.StatusBadRequest, &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	var resp VerifyIntegrationResponse

	switch req.Provider {
	case "telegram":
		resp = verifyTelegram(ctx, req.Config)
	case "feishu":
		resp = verifyFeishu(ctx, req.Config)
	case "dingtalk":
		resp = verifyDingtalk(ctx, req.Config)
	case "discord":
		resp = verifyDiscord(ctx, req.Config)
	default:
		response.RespondWithError(c, response.StatusBadRequest, &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: fmt.Sprintf("unsupported provider: %s", req.Provider),
		})
		return
	}

	response.RespondWithSuccess(c, response.StatusOK, resp)
}

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func verifyTelegram(ctx context.Context, cfg map[string]any) VerifyIntegrationResponse {
	token := str(cfg, "bot_token")
	if token == "" {
		return VerifyIntegrationResponse{Valid: false, Error: "bot_token is required"}
	}

	var opts []telegram.BotOption
	if host := str(cfg, "host"); host != "" {
		opts = append(opts, telegram.WithAPIBase(host))
	}

	bot := telegram.NewBot(token, "", opts...)
	user, err := bot.GetMe(ctx)
	if err != nil {
		return VerifyIntegrationResponse{Valid: false, Error: err.Error()}
	}

	info := map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"name":     user.FirstName,
	}
	if user.LastName != "" {
		info["name"] = user.FirstName + " " + user.LastName
	}
	return VerifyIntegrationResponse{Valid: true, Info: info}
}

func verifyFeishu(ctx context.Context, cfg map[string]any) VerifyIntegrationResponse {
	appID := str(cfg, "app_id")
	appSecret := str(cfg, "app_secret")
	if appID == "" || appSecret == "" {
		return VerifyIntegrationResponse{Valid: false, Error: "app_id and app_secret are required"}
	}

	bot := feishu.NewBot(appID, appSecret)

	resp, err := bot.Client().GetTenantAccessTokenBySelfBuiltApp(ctx,
		&larkcore.SelfBuiltTenantAccessTokenReq{AppID: appID, AppSecret: appSecret})
	if err != nil {
		return VerifyIntegrationResponse{Valid: false, Error: err.Error()}
	}
	if resp == nil || !resp.Success() {
		msg := "failed to obtain tenant access token"
		if resp != nil {
			msg = fmt.Sprintf("code=%d msg=%s", resp.Code, resp.Msg)
		}
		return VerifyIntegrationResponse{Valid: false, Error: msg}
	}

	return VerifyIntegrationResponse{
		Valid: true,
		Info: map[string]any{
			"app_id": appID,
			"status": "credentials_valid",
		},
	}
}

func verifyDingtalk(ctx context.Context, cfg map[string]any) VerifyIntegrationResponse {
	clientID := str(cfg, "client_id")
	clientSecret := str(cfg, "client_secret")
	if clientID == "" || clientSecret == "" {
		return VerifyIntegrationResponse{Valid: false, Error: "client_id and client_secret are required"}
	}

	bot := dingtalk.NewBot(clientID, clientSecret)
	if err := bot.GetBotInfo(ctx); err != nil {
		return VerifyIntegrationResponse{Valid: false, Error: err.Error()}
	}

	return VerifyIntegrationResponse{
		Valid: true,
		Info: map[string]any{
			"client_id": clientID,
			"status":    "credentials_valid",
		},
	}
}

func verifyDiscord(ctx context.Context, cfg map[string]any) VerifyIntegrationResponse {
	botToken := str(cfg, "bot_token")
	if botToken == "" {
		return VerifyIntegrationResponse{Valid: false, Error: "bot_token is required"}
	}

	appID := str(cfg, "app_id")
	bot, err := discord.NewBot(botToken, appID)
	if err != nil {
		return VerifyIntegrationResponse{Valid: false, Error: err.Error()}
	}

	user, err := bot.BotUser()
	if err != nil {
		return VerifyIntegrationResponse{Valid: false, Error: err.Error()}
	}

	return VerifyIntegrationResponse{
		Valid: true,
		Info: map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"name":     user.GlobalName,
		},
	}
}
