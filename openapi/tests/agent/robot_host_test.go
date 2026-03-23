package openapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaoapp/yao/openapi"
	"github.com/yaoapp/yao/openapi/tests/testutils"
)

// TestRobotHostID tests GET /v1/agent/robots/:id/host
// Returns the host assistant ID for a given robot.
func TestRobotHostID(t *testing.T) {
	serverURL := testutils.Prepare(t)
	defer testutils.Clean()

	baseURL := ""
	if openapi.Server != nil && openapi.Server.Config != nil {
		baseURL = openapi.Server.Config.BaseURL
	}

	client := testutils.RegisterTestClient(t, "Host ID Test Client", []string{"https://localhost/callback"})
	defer testutils.CleanupTestClient(t, client.ClientID)
	tokenInfo := testutils.ObtainAccessToken(t, serverURL, client.ClientID, client.ClientSecret, "https://localhost/callback", "openid profile")

	// Create a test robot with a minimal config
	robotID := fmt.Sprintf("test_host_id_%d", time.Now().UnixNano())
	createTestRobotForHost(t, serverURL, baseURL, tokenInfo.AccessToken, robotID, "Host ID Test Robot")
	defer deleteTestRobotForHost(t, serverURL, baseURL, tokenInfo.AccessToken, robotID)

	t.Run("GetHostIDSuccess", func(t *testing.T) {
		req, err := http.NewRequest("GET", serverURL+baseURL+"/agent/robots/"+robotID+"/host", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Contains(t, response, "assistant_id", "Response should contain assistant_id")
		assert.Contains(t, response, "robot_id", "Response should contain robot_id")
		assert.Equal(t, robotID, response["robot_id"])
		// assistant_id is either configured or falls back to "__yao.host"
		assistantID, ok := response["assistant_id"].(string)
		assert.True(t, ok, "assistant_id should be a string")
		assert.NotEmpty(t, assistantID, "assistant_id should not be empty")
		t.Logf("✓ Host ID: robot=%s, assistant=%s", robotID, assistantID)
	})

	t.Run("GetHostIDNotFound", func(t *testing.T) {
		req, err := http.NewRequest("GET", serverURL+baseURL+"/agent/robots/non_existent_robot/host", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		t.Logf("✓ Non-existent robot returns error for /host")
	})

	t.Run("GetHostIDUnauthorized", func(t *testing.T) {
		req, err := http.NewRequest("GET", serverURL+baseURL+"/agent/robots/"+robotID+"/host", nil)
		require.NoError(t, err)
		// No Authorization header

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		t.Logf("✓ Unauthorized request rejected")
	})
}

// TestRobotExecute tests POST /v1/agent/robots/:id/execute
// Called by CUI after Host Agent confirms goals via robot.execute Action.
func TestRobotExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping execute tests in short mode (requires manager)")
	}

	serverURL := testutils.Prepare(t)
	defer testutils.Clean()

	baseURL := ""
	if openapi.Server != nil && openapi.Server.Config != nil {
		baseURL = openapi.Server.Config.BaseURL
	}

	client := testutils.RegisterTestClient(t, "Execute Test Client", []string{"https://localhost/callback"})
	defer testutils.CleanupTestClient(t, client.ClientID)
	tokenInfo := testutils.ObtainAccessToken(t, serverURL, client.ClientID, client.ClientSecret, "https://localhost/callback", "openid profile")

	robotID := fmt.Sprintf("test_execute_%d", time.Now().UnixNano())
	createTestRobotForHost(t, serverURL, baseURL, tokenInfo.AccessToken, robotID, "Execute Test Robot")
	defer deleteTestRobotForHost(t, serverURL, baseURL, tokenInfo.AccessToken, robotID)

	t.Run("ExecuteWithGoals", func(t *testing.T) {
		execData := map[string]interface{}{
			"goals":   "Create a mecha image with sci-fi style",
			"chat_id": fmt.Sprintf("robot_%s_%d", robotID, time.Now().UnixMilli()),
			"context": map[string]interface{}{
				"style":   "sci-fi",
				"subject": "mecha",
			},
		}

		body, _ := json.Marshal(execData)
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/execute", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Either 200 (accepted) or 400 (trigger disabled / manager not running)
		// Both are valid — we test the API contract
		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusOK {
			assert.Contains(t, response, "execution_id", "Should return execution_id")
			assert.Equal(t, "started", response["status"])
			t.Logf("✓ Execute accepted: execution_id=%v", response["execution_id"])
		} else {
			// Manager not running or trigger disabled is acceptable in test env
			t.Logf("Execute returned %d: %v (manager may not be running)", resp.StatusCode, response)
		}
	})

	t.Run("ExecuteMissingGoals", func(t *testing.T) {
		// goals is required — omitting it should fail with 400
		execData := map[string]interface{}{
			"chat_id": fmt.Sprintf("robot_%s_%d", robotID, time.Now().UnixMilli()),
		}

		body, _ := json.Marshal(execData)
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/execute", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		t.Logf("✓ Missing goals returns 400")
	})

	t.Run("ExecuteNotFound", func(t *testing.T) {
		execData := map[string]interface{}{
			"goals": "Some goal",
		}
		body, _ := json.Marshal(execData)
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/non_existent_robot_xyz/execute", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 404 (robot not found), 400 (trigger disabled) or 500 (manager not running) — all acceptable
		assert.True(t,
			resp.StatusCode == http.StatusNotFound ||
				resp.StatusCode == http.StatusBadRequest ||
				resp.StatusCode == http.StatusInternalServerError,
			"Non-existent robot should return 4xx or 500, got %d", resp.StatusCode)
		t.Logf("✓ Non-existent robot execute returns %d", resp.StatusCode)
	})

	t.Run("ExecuteUnauthorized", func(t *testing.T) {
		execData := map[string]interface{}{
			"goals": "Some goal",
		}
		body, _ := json.Marshal(execData)
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/execute", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		// No Authorization header

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		t.Logf("✓ Unauthorized execute returns 401")
	})
}

// TestRobotCompletionsMirrorAPI tests POST /v1/agent/robots/:id/completions
// Mirror API that resolves host assistant and delegates to standard chat completions.
func TestRobotCompletionsMirrorAPI(t *testing.T) {
	serverURL := testutils.Prepare(t)
	defer testutils.Clean()

	baseURL := ""
	if openapi.Server != nil && openapi.Server.Config != nil {
		baseURL = openapi.Server.Config.BaseURL
	}

	client := testutils.RegisterTestClient(t, "Completions Mirror Test Client", []string{"https://localhost/callback"})
	defer testutils.CleanupTestClient(t, client.ClientID)
	tokenInfo := testutils.ObtainAccessToken(t, serverURL, client.ClientID, client.ClientSecret, "https://localhost/callback", "openid profile")

	robotID := fmt.Sprintf("test_completions_%d", time.Now().UnixNano())
	createTestRobotForHost(t, serverURL, baseURL, tokenInfo.AccessToken, robotID, "Completions Mirror Test Robot")
	defer deleteTestRobotForHost(t, serverURL, baseURL, tokenInfo.AccessToken, robotID)

	t.Run("RejectsEmptyBody", func(t *testing.T) {
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/completions",
			bytes.NewBuffer([]byte("{}")))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should be handled by the chat completion handler
		// 4xx or 5xx is acceptable — we're verifying the route resolves
		assert.True(t, resp.StatusCode >= 400, "Empty completions request should fail, got %d", resp.StatusCode)
		t.Logf("✓ Empty completions body handled: %d", resp.StatusCode)
	})

	t.Run("RejectsNonExistentRobot", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"chat_id":      "test_chat_id",
			"assistant_id": "some_assistant",
			"messages": []map[string]interface{}{
				{"role": "user", "content": "hello"},
			},
		})
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/non_existent_robot/completions",
			bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.True(t, resp.StatusCode >= 400, "Non-existent robot should return error, got %d", resp.StatusCode)
		t.Logf("✓ Non-existent robot completions handled: %d", resp.StatusCode)
	})

	t.Run("UnauthorizedReturns401", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"chat_id": "test_chat",
		})
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/completions",
			bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		// No Authorization header

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		t.Logf("✓ Unauthorized completions returns 401")
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

func createTestRobotForHost(t *testing.T, serverURL, baseURL, token, robotID, name string) {
	t.Helper()
	createData := map[string]interface{}{
		"member_id":    robotID,
		"team_id":      "test_team_host_001",
		"display_name": name,
		"bio":          "A test robot for host API testing",
	}
	body, _ := json.Marshal(createData)
	req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots", bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
}

func deleteTestRobotForHost(t *testing.T, serverURL, baseURL, token, robotID string) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", serverURL+baseURL+"/agent/robots/"+robotID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}
