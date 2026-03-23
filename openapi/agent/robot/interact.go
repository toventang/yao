package robot

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/agent/output/message"
	robotapi "github.com/yaoapp/yao/agent/robot/api"
	robottypes "github.com/yaoapp/yao/agent/robot/types"
	"github.com/yaoapp/yao/openapi/oauth/authorized"
	"github.com/yaoapp/yao/openapi/response"
)

// InteractRequest - HTTP request for unified robot interaction
type InteractRequest struct {
	ExecutionID string `json:"execution_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Source      string `json:"source,omitempty"`
	Message     string `json:"message" binding:"required"`
	Action      string `json:"action,omitempty"`
	Stream      bool   `json:"stream,omitempty"`
}

// InteractResponse - HTTP response for interaction
type InteractResponse struct {
	ExecutionID string `json:"execution_id,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
	Reply       string `json:"reply,omitempty"`
	WaitForMore bool   `json:"wait_for_more,omitempty"`
}

// ReplyRequest - HTTP request for replying to a waiting task
type ReplyRequest struct {
	Message string `json:"message" binding:"required"`
}

// ConfirmRequest - HTTP request for confirming an execution
type ConfirmRequest struct {
	Message string `json:"message,omitempty"`
}

// InteractRobot handles unified robot interaction
// POST /v1/agent/robots/:id/interact
func InteractRobot(c *gin.Context) {
	authInfo := authorized.GetInfo(c)
	if authInfo == nil || (authInfo.Subject == "" && authInfo.UserID == "") {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidToken.Code,
			ErrorDescription: "Authentication required",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}
	robotID := c.Param("id")
	if robotID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "robot id is required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	var req InteractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Invalid request body: " + err.Error(),
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	ctx := robottypes.NewContext(c.Request.Context(), authInfo)
	robotResp, err := robotapi.GetRobotResponse(ctx, robotID)
	if err != nil {
		if errors.Is(err, robottypes.ErrRobotNotFound) {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: "Robot not found: " + robotID,
			}
			response.RespondWithError(c, response.StatusNotFound, errorResp)
			return
		}
		errorResp := &response.ErrorResponse{
			Code:             response.ErrServerError.Code,
			ErrorDescription: "Failed to get robot: " + err.Error(),
		}
		response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		return
	}

	if !CanWrite(c, authInfo, robotResp.YaoTeamID, robotResp.YaoCreatedBy) {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrAccessDenied.Code,
			ErrorDescription: "Forbidden: No permission to interact with this robot",
		}
		response.RespondWithError(c, response.StatusForbidden, errorResp)
		return
	}

	apiReq := &robotapi.InteractRequest{
		ExecutionID: req.ExecutionID,
		TaskID:      req.TaskID,
		Source:      robottypes.InteractSource(req.Source),
		Message:     req.Message,
		Action:      req.Action,
	}

	// Detect SSE mode: request body stream=true or Accept header
	wantSSE := req.Stream || c.GetHeader("Accept") == "text/event-stream"

	if wantSSE {
		interactSSE(c, ctx, robotID, apiReq)
		return
	}

	result, err := robotapi.Interact(ctx, robotID, apiReq)
	if err != nil {
		log.Error("Failed to interact with robot %s: %v", robotID, err)
		errorResp := &response.ErrorResponse{
			Code:             response.ErrServerError.Code,
			ErrorDescription: "Failed to interact: " + err.Error(),
		}
		response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		return
	}

	resp := &InteractResponse{
		ExecutionID: result.ExecutionID,
		Status:      result.Status,
		Message:     result.Message,
		ChatID:      result.ChatID,
		Reply:       result.Reply,
		WaitForMore: result.WaitForMore,
	}
	response.RespondWithSuccess(c, response.StatusOK, resp)
}

// interactSSE handles the SSE streaming mode for robot interaction.
// Outputs standard CUI Message protocol (data: {json}\n\n) for direct frontend consumption,
// plus a final "interact_done" event with execution metadata.
func interactSSE(c *gin.Context, ctx *robottypes.Context, robotID string, apiReq *robotapi.InteractRequest) {
	c.Header("Content-Type", "text/event-stream;charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	w := c.Writer
	flusher, ok := w.(interface{ Flush() })
	if !ok {
		log.Error("ResponseWriter does not support Flush")
		return
	}

	writeData := func(data interface{}) {
		raw, err := json.Marshal(data)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", raw)
		flusher.Flush()
	}

	onMessage := func(msg *message.Message) int {
		if msg == nil {
			return 0
		}
		writeData(msg)
		return 0
	}

	result, err := robotapi.InteractStreamRaw(ctx, robotID, apiReq, onMessage)
	if err != nil {
		writeData(&message.Message{
			Type: message.TypeError,
			Props: map[string]interface{}{
				"message": err.Error(),
			},
		})
		writeData(&message.Message{
			Type: message.TypeEvent,
			Props: map[string]interface{}{
				"event":   "interact_done",
				"message": "error",
				"data": map[string]interface{}{
					"status": "error",
					"error":  err.Error(),
				},
			},
		})
		return
	}

	writeData(&message.Message{
		Type: message.TypeEvent,
		Props: map[string]interface{}{
			"event":   "interact_done",
			"message": result.Message,
			"data": map[string]interface{}{
				"execution_id":  result.ExecutionID,
				"status":        result.Status,
				"message":       result.Message,
				"chat_id":       result.ChatID,
				"reply":         result.Reply,
				"wait_for_more": result.WaitForMore,
			},
		},
	})
}

// ReplyToTask handles replying to a specific waiting task
// POST /v1/agent/robots/:id/executions/:exec_id/tasks/:task_id/reply
func ReplyToTask(c *gin.Context) {
	authInfo := authorized.GetInfo(c)
	if authInfo == nil || (authInfo.Subject == "" && authInfo.UserID == "") {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidToken.Code,
			ErrorDescription: "Authentication required",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}
	robotID := c.Param("id")
	execID := c.Param("exec_id")
	taskID := c.Param("task_id")

	if robotID == "" || execID == "" || taskID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "robot id, execution id, and task id are required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	var req ReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Invalid request body: " + err.Error(),
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	ctx := robottypes.NewContext(c.Request.Context(), authInfo)
	robotResp, err := robotapi.GetRobotResponse(ctx, robotID)
	if err != nil {
		handleRobotError(c, robotID, err)
		return
	}

	if !CanWrite(c, authInfo, robotResp.YaoTeamID, robotResp.YaoCreatedBy) {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrAccessDenied.Code,
			ErrorDescription: "Forbidden",
		}
		response.RespondWithError(c, response.StatusForbidden, errorResp)
		return
	}

	result, err := robotapi.Reply(ctx, robotID, execID, taskID, req.Message)
	if err != nil {
		log.Error("Failed to reply to task: %v", err)
		errorResp := &response.ErrorResponse{
			Code:             response.ErrServerError.Code,
			ErrorDescription: "Failed to reply: " + err.Error(),
		}
		response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		return
	}

	resp := &InteractResponse{
		ExecutionID: result.ExecutionID,
		Status:      result.Status,
		Message:     result.Message,
	}
	response.RespondWithSuccess(c, response.StatusOK, resp)
}

// ConfirmExecution handles confirming a pending execution
// POST /v1/agent/robots/:id/executions/:exec_id/confirm
func ConfirmExecution(c *gin.Context) {
	authInfo := authorized.GetInfo(c)
	if authInfo == nil || (authInfo.Subject == "" && authInfo.UserID == "") {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidToken.Code,
			ErrorDescription: "Authentication required",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}
	robotID := c.Param("id")
	execID := c.Param("exec_id")

	if robotID == "" || execID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "robot id and execution id are required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body for confirm
		req = ConfirmRequest{}
	}

	ctx := robottypes.NewContext(c.Request.Context(), authInfo)
	robotResp, err := robotapi.GetRobotResponse(ctx, robotID)
	if err != nil {
		handleRobotError(c, robotID, err)
		return
	}

	if !CanWrite(c, authInfo, robotResp.YaoTeamID, robotResp.YaoCreatedBy) {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrAccessDenied.Code,
			ErrorDescription: "Forbidden",
		}
		response.RespondWithError(c, response.StatusForbidden, errorResp)
		return
	}

	result, err := robotapi.Confirm(ctx, robotID, execID, req.Message)
	if err != nil {
		log.Error("Failed to confirm execution: %v", err)
		errorResp := &response.ErrorResponse{
			Code:             response.ErrServerError.Code,
			ErrorDescription: "Failed to confirm: " + err.Error(),
		}
		response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		return
	}

	resp := &InteractResponse{
		ExecutionID: result.ExecutionID,
		Status:      result.Status,
		Message:     result.Message,
	}
	response.RespondWithSuccess(c, response.StatusOK, resp)
}

func handleRobotError(c *gin.Context, robotID string, err error) {
	if errors.Is(err, robottypes.ErrRobotNotFound) {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Robot not found: " + robotID,
		}
		response.RespondWithError(c, response.StatusNotFound, errorResp)
		return
	}
	errorResp := &response.ErrorResponse{
		Code:             response.ErrServerError.Code,
		ErrorDescription: "Failed to get robot: " + err.Error(),
	}
	response.RespondWithError(c, response.StatusInternalServerError, errorResp)
}
