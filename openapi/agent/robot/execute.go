package robot

import (
	"errors"

	"github.com/gin-gonic/gin"
	robotapi "github.com/yaoapp/yao/agent/robot/api"
	robottypes "github.com/yaoapp/yao/agent/robot/types"
	"github.com/yaoapp/yao/openapi/oauth/authorized"
	"github.com/yaoapp/yao/openapi/response"
)

// ExecuteRequest is the request body for POST /v1/agent/robots/:id/execute
type ExecuteRequest struct {
	Goals   string                 `json:"goals" binding:"required"`
	Context map[string]interface{} `json:"context,omitempty"`
	ChatID  string                 `json:"chat_id,omitempty"`
}

// ExecuteRobot handles POST /v1/agent/robots/:id/execute
// Directly triggers robot execution with confirmed goals, bypassing Host Agent conversation.
// Called by CUI after the Host Agent's NEXT HOOK sends a robot.execute Action.
func ExecuteRobot(c *gin.Context) {
	authInfo := authorized.GetInfo(c)
	if authInfo == nil || (authInfo.Subject == "" && authInfo.UserID == "") {
		response.RespondWithError(c, response.StatusUnauthorized, &response.ErrorResponse{
			Code:             response.ErrInvalidToken.Code,
			ErrorDescription: "Authentication required",
		})
		return
	}

	robotID := c.Param("id")
	if robotID == "" {
		response.RespondWithError(c, response.StatusBadRequest, &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "robot id is required",
		})
		return
	}

	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.RespondWithError(c, response.StatusBadRequest, &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := robottypes.NewContext(c.Request.Context(), authInfo)

	// Build TriggerInput with confirmed goals from Host Agent.
	// Passing goals via Data["goals"] allows RunGoals to skip the Goals Agent
	// and use the pre-confirmed goals directly.
	data := map[string]interface{}{
		"goals": req.Goals,
	}
	if req.Context != nil {
		data["context"] = req.Context
	}
	if req.ChatID != "" {
		data["chat_id"] = req.ChatID
	}
	triggerInput := &robottypes.TriggerInput{
		Data: data,
	}

	result, err := robotapi.TriggerManual(ctx, robotID, robottypes.TriggerHuman, triggerInput)
	if err != nil {
		if errors.Is(err, robottypes.ErrRobotNotFound) {
			response.RespondWithError(c, response.StatusNotFound, &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: "Robot not found: " + robotID,
			})
			return
		}
		response.RespondWithError(c, response.StatusInternalServerError, &response.ErrorResponse{
			Code:             response.ErrServerError.Code,
			ErrorDescription: "Failed to execute: " + err.Error(),
		})
		return
	}

	if !result.Accepted {
		response.RespondWithError(c, response.StatusBadRequest, &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: result.Message,
		})
		return
	}

	response.RespondWithSuccess(c, response.StatusOK, gin.H{
		"execution_id": result.ExecutionID,
		"status":       "started",
		"message":      result.Message,
	})
}
