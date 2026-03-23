package robot

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaoapp/yao/openapi/response"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setAuthContext sets the context values that authorized.GetInfo(c) reads
func setAuthContext(c *gin.Context) {
	c.Set("__subject", "test-subject")
	c.Set("__client_id", "test-client")
	c.Set("__user_id", "test-user")
	c.Set("__scope", "openid profile")
}

// OH1: InteractRobot with missing auth info
func TestInteractRobot_OH1_MissingAuth(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "robot-123"}}
	body := bytes.NewBufferString(`{"message":"hello"}`)
	c.Request, _ = http.NewRequest("POST", "/v1/agent/robots/robot-123/interact", body)
	c.Request.Header.Set("Content-Type", "application/json")
	// No auth context set

	InteractRobot(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidToken.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "Authentication")
}

// OH2: InteractRobot with invalid JSON body
func TestInteractRobot_OH2_InvalidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAuthContext(c)
	c.Params = gin.Params{{Key: "id", Value: "robot-123"}}
	body := bytes.NewBufferString(`{invalid json`)
	c.Request, _ = http.NewRequest("POST", "/v1/agent/robots/robot-123/interact", body)
	c.Request.Header.Set("Content-Type", "application/json")

	InteractRobot(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidRequest.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "Invalid request body")
}

// OH3: InteractRobot empty message validation
func TestInteractRobot_OH3_EmptyMessage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAuthContext(c)
	c.Params = gin.Params{{Key: "id", Value: "robot-123"}}
	body := bytes.NewBufferString(`{}`)
	c.Request, _ = http.NewRequest("POST", "/v1/agent/robots/robot-123/interact", body)
	c.Request.Header.Set("Content-Type", "application/json")

	InteractRobot(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidRequest.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "Invalid request body")
}

// OH4: ReplyToTask with missing auth info
func TestReplyToTask_OH4_MissingAuth(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "robot-123"},
		{Key: "exec_id", Value: "exec-456"},
		{Key: "task_id", Value: "task-789"},
	}
	body := bytes.NewBufferString(`{"message":"reply"}`)
	c.Request, _ = http.NewRequest("POST", "/v1/agent/robots/robot-123/executions/exec-456/tasks/task-789/reply", body)
	c.Request.Header.Set("Content-Type", "application/json")
	// No auth context set

	ReplyToTask(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidToken.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "Authentication")
}

// OH5: ReplyToTask with empty robot_id parameter
func TestReplyToTask_OH5_EmptyRobotID(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAuthContext(c)
	c.Params = gin.Params{
		{Key: "id", Value: ""},
		{Key: "exec_id", Value: "exec-456"},
		{Key: "task_id", Value: "task-789"},
	}
	body := bytes.NewBufferString(`{"message":"reply"}`)
	c.Request, _ = http.NewRequest("POST", "/reply", body)
	c.Request.Header.Set("Content-Type", "application/json")

	ReplyToTask(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidRequest.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "robot id")
}

// OH6: ReplyToTask with empty message
func TestReplyToTask_OH6_EmptyMessage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAuthContext(c)
	c.Params = gin.Params{
		{Key: "id", Value: "robot-123"},
		{Key: "exec_id", Value: "exec-456"},
		{Key: "task_id", Value: "task-789"},
	}
	body := bytes.NewBufferString(`{}`)
	c.Request, _ = http.NewRequest("POST", "/reply", body)
	c.Request.Header.Set("Content-Type", "application/json")

	ReplyToTask(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidRequest.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "Invalid request body")
}

// OH7: ConfirmExecution with missing auth info
func TestConfirmExecution_OH7_MissingAuth(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "robot-123"},
		{Key: "exec_id", Value: "exec-456"},
	}
	body := bytes.NewBufferString(`{}`)
	c.Request, _ = http.NewRequest("POST", "/v1/agent/robots/robot-123/executions/exec-456/confirm", body)
	c.Request.Header.Set("Content-Type", "application/json")
	// No auth context set

	ConfirmExecution(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidToken.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "Authentication")
}

// OH8: ConfirmExecution with empty execution_id
func TestConfirmExecution_OH8_EmptyExecutionID(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAuthContext(c)
	c.Params = gin.Params{
		{Key: "id", Value: "robot-123"},
		{Key: "exec_id", Value: ""},
	}
	body := bytes.NewBufferString(`{}`)
	c.Request, _ = http.NewRequest("POST", "/confirm", body)
	c.Request.Header.Set("Content-Type", "application/json")

	ConfirmExecution(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidRequest.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "execution id")
}

// OH9: ConfirmExecution with valid request (robot not found expected)
// Requires app/database to be initialized; skipped in short mode.
func TestConfirmExecution_OH9_RobotNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping OH9 in short mode: requires app/database for GetRobotResponse")
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	setAuthContext(c)
	c.Params = gin.Params{
		{Key: "id", Value: "non-existent-robot-999"},
		{Key: "exec_id", Value: "exec-456"},
	}
	body := bytes.NewBufferString(`{}`)
	c.Request, _ = http.NewRequest("POST", "/v1/agent/robots/non-existent-robot-999/executions/exec-456/confirm", body)
	c.Request.Header.Set("Content-Type", "application/json")

	ConfirmExecution(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	var errResp response.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, response.ErrInvalidRequest.Code, errResp.Code)
	assert.Contains(t, errResp.ErrorDescription, "Robot not found")
	assert.Contains(t, errResp.ErrorDescription, "non-existent-robot-999")
}
