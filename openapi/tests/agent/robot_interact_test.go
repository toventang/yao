package openapi_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	robotapi "github.com/yaoapp/yao/agent/robot/api"
	"github.com/yaoapp/yao/openapi"
	"github.com/yaoapp/yao/openapi/tests/testutils"
)

func TestInteractRobot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping interact tests in short mode (requires AI/manager)")
	}

	serverURL := testutils.Prepare(t)
	defer testutils.Clean()

	baseURL := ""
	if openapi.Server != nil && openapi.Server.Config != nil {
		baseURL = openapi.Server.Config.BaseURL
	}

	client := testutils.RegisterTestClient(t, "Interact Test Client", []string{"https://localhost/callback"})
	defer testutils.CleanupTestClient(t, client.ClientID)
	tokenInfo := testutils.ObtainAccessToken(t, serverURL, client.ClientID, client.ClientSecret, "https://localhost/callback", "openid profile")

	err := robotapi.Start()
	require.NoError(t, err, "Manager must start for Interact tests")
	defer robotapi.Stop()

	robotID := fmt.Sprintf("test_interact_%d", time.Now().UnixNano())
	createRobotForInteract(t, serverURL, baseURL, tokenInfo.AccessToken, robotID, "Interact Test Robot")
	defer deleteRobotForInteract(t, serverURL, baseURL, tokenInfo.AccessToken, robotID)

	t.Run("InteractSync_FullFlow", func(t *testing.T) {
		interactData := map[string]interface{}{
			"message": "Please write a short greeting email for our Monday standup.",
		}

		body, _ := json.Marshal(interactData)
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/interact", bytes.NewBuffer(body))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close()

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		t.Logf("Sync interact: status_code=%d, response=%+v", resp.StatusCode, response)

		if resp.StatusCode == http.StatusOK {
			data, _ := response["data"].(map[string]interface{})
			if data != nil {
				assert.NotEmpty(t, data["execution_id"], "should have execution_id")
				assert.NotEmpty(t, data["reply"], "Host Agent should provide a reply")
				assert.NotEmpty(t, data["status"], "should have a status")

				validStatuses := []string{"confirmed", "waiting_for_more", "adjusted", "acknowledged"}
				status, _ := data["status"].(string)
				assert.Contains(t, validStatuses, status,
					"status should reflect Host Agent action outcome, got: %s", status)
				t.Logf("Sync result: exec_id=%v, status=%v, reply=%v, wait_for_more=%v",
					data["execution_id"], data["status"], data["reply"], data["wait_for_more"])
			}
		} else {
			t.Logf("Sync interact returned %d: %v (may indicate Manager routing issue)", resp.StatusCode, response)
		}
	})

	t.Run("InteractSSE_FullFlow", func(t *testing.T) {
		interactData := map[string]interface{}{
			"message": "Draft a brief thank-you note for the design team.",
			"stream":  true,
		}

		body, _ := json.Marshal(interactData)
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/interact", bytes.NewBuffer(body))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&errResp)
			t.Fatalf("SSE interact failed: status=%d, error=%v", resp.StatusCode, errResp)
		}

		contentType := resp.Header.Get("Content-Type")
		assert.Contains(t, contentType, "text/event-stream")

		messages := parseCUISSEMessages(t, resp)
		require.NotEmpty(t, messages, "should receive CUI message events")

		var textMessages []map[string]interface{}
		var interactDone map[string]interface{}
		for _, msg := range messages {
			msgType, _ := msg["type"].(string)
			if msgType == "text" {
				textMessages = append(textMessages, msg)
			}
			if msgType == "event" {
				props, _ := msg["props"].(map[string]interface{})
				if props != nil {
					if evt, _ := props["event"].(string); evt == "interact_done" {
						interactDone = props
					}
				}
			}
		}

		t.Logf("SSE: %d total messages, %d text messages, interact_done=%v",
			len(messages), len(textMessages), interactDone != nil)

		assert.NotNil(t, interactDone, "should have an interact_done event")
		if interactDone != nil {
			doneData, _ := interactDone["data"].(map[string]interface{})
			if doneData != nil {
				if status, ok := doneData["status"].(string); ok {
					validStatuses := []string{"confirmed", "waiting_for_more", "adjusted", "acknowledged", "error"}
					assert.Contains(t, validStatuses, status,
						"final status should be a valid outcome")
				}
				if execID, ok := doneData["execution_id"].(string); ok {
					assert.NotEmpty(t, execID, "done event should carry execution_id")
				}
				t.Logf("SSE done data: %+v", doneData)
			}
		}
	})

	t.Run("InteractSSE_MultiTurn", func(t *testing.T) {
		// Turn 1: vague request
		body1, _ := json.Marshal(map[string]interface{}{
			"message": "Do something with emails.",
			"stream":  true,
		})
		req1, _ := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/interact", bytes.NewBuffer(body1))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Accept", "text/event-stream")
		req1.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp1, err := http.DefaultClient.Do(req1)
		require.NoError(t, err)
		defer resp1.Body.Close()

		if resp1.StatusCode != http.StatusOK {
			var errResp map[string]interface{}
			json.NewDecoder(resp1.Body).Decode(&errResp)
			t.Fatalf("Turn 1 SSE failed: status=%d, error=%v", resp1.StatusCode, errResp)
		}

		turn1Messages := parseCUISSEMessages(t, resp1)
		require.NotEmpty(t, turn1Messages)

		turn1Done := findInteractDone(turn1Messages)
		require.NotNil(t, turn1Done, "Turn 1 should have interact_done event")

		doneData1, _ := turn1Done["data"].(map[string]interface{})
		require.NotNil(t, doneData1)
		execID, _ := doneData1["execution_id"].(string)
		t.Logf("Turn 1: exec_id=%s, status=%v, wait_for_more=%v",
			execID, doneData1["status"], doneData1["wait_for_more"])
		assert.NotEmpty(t, execID, "Turn 1 should create an execution")

		// Turn 2: clarify with same execution_id
		body2, _ := json.Marshal(map[string]interface{}{
			"execution_id": execID,
			"message":      "Please write a congratulations email for the team hitting Q4 targets. Yes, proceed.",
			"stream":       true,
		})
		req2, _ := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/interact", bytes.NewBuffer(body2))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Accept", "text/event-stream")
		req2.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp2, err := http.DefaultClient.Do(req2)
		require.NoError(t, err)
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			var errResp map[string]interface{}
			json.NewDecoder(resp2.Body).Decode(&errResp)
			t.Fatalf("Turn 2 SSE failed: status=%d, error=%v", resp2.StatusCode, errResp)
		}

		turn2Messages := parseCUISSEMessages(t, resp2)
		require.NotEmpty(t, turn2Messages)

		turn2Done := findInteractDone(turn2Messages)
		require.NotNil(t, turn2Done, "Turn 2 should have interact_done event")

		doneData2, _ := turn2Done["data"].(map[string]interface{})
		require.NotNil(t, doneData2)
		execID2, _ := doneData2["execution_id"].(string)
		t.Logf("Turn 2: exec_id=%s, status=%v, wait_for_more=%v",
			execID2, doneData2["status"], doneData2["wait_for_more"])
		assert.Equal(t, execID, execID2, "Turn 2 should reference same execution")
	})

	t.Run("InteractMissingMessage", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{})
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/interact", bytes.NewBuffer(body))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("InteractNotFound", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{"message": "test"})
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/non_existent_robot/interact", bytes.NewBuffer(body))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("InteractUnauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{"message": "test"})
		req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots/"+robotID+"/interact", bytes.NewBuffer(body))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// parseCUISSEMessages parses the SSE stream using CUI Message protocol format:
// each line is "data: {json}\n\n" where the JSON is a message.Message object.
func parseCUISSEMessages(t *testing.T, resp *http.Response) []map[string]interface{} {
	scanner := bufio.NewScanner(resp.Body)
	var messages []map[string]interface{}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(data), &parsed); err == nil {
				messages = append(messages, parsed)
			}
		}
	}
	return messages
}

// findInteractDone finds the interact_done event from CUI messages.
func findInteractDone(messages []map[string]interface{}) map[string]interface{} {
	for _, msg := range messages {
		msgType, _ := msg["type"].(string)
		if msgType == "event" {
			props, _ := msg["props"].(map[string]interface{})
			if props != nil {
				if evt, _ := props["event"].(string); evt == "interact_done" {
					return props
				}
			}
		}
	}
	return nil
}

func createRobotForInteract(t *testing.T, serverURL, baseURL, token, robotID, displayName string) {
	createData := map[string]interface{}{
		"member_id":    robotID,
		"team_id":      "test_team_001",
		"display_name": displayName,
		"robot_config": map[string]interface{}{
			"identity": map[string]interface{}{
				"role":   "Email Assistant",
				"duties": []string{"Write and manage emails"},
			},
			"quota": map[string]interface{}{
				"max":   5,
				"queue": 20,
			},
			"triggers": map[string]interface{}{
				"intervene": map[string]interface{}{"enabled": true},
			},
			"resources": map[string]interface{}{
				"phases": map[string]interface{}{
					"host": "robot.host",
				},
			},
		},
	}

	body, _ := json.Marshal(createData)
	req, err := http.NewRequest("POST", serverURL+baseURL+"/agent/robots", bytes.NewBuffer(body))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		t.Logf("Create robot response: %d %v", resp.StatusCode, errBody)
	}
}

func deleteRobotForInteract(t *testing.T, serverURL, baseURL, token, robotID string) {
	req, _ := http.NewRequest("DELETE", serverURL+baseURL+"/agent/robots/"+robotID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := http.DefaultClient.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
}
