package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostInputJSON(t *testing.T) {
	input := &HostInput{
		Scenario: "assign",
		Context: &HostContext{
			RobotStatus: &RobotStatusSnapshot{
				ActiveCount: 1,
				MaxQuota:    5,
			},
			Goals: &Goals{Content: "test goals"},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var parsed HostInput
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "assign", parsed.Scenario)
	assert.NotNil(t, parsed.Context)
	assert.Equal(t, 1, parsed.Context.RobotStatus.ActiveCount)
}

func TestHostOutputJSON(t *testing.T) {
	output := &HostOutput{
		Reply:       "Task confirmed",
		Action:      HostActionConfirm,
		WaitForMore: false,
	}

	data, err := json.Marshal(output)
	require.NoError(t, err)

	var parsed HostOutput
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "Task confirmed", parsed.Reply)
	assert.Equal(t, HostActionConfirm, parsed.Action)
	assert.False(t, parsed.WaitForMore)
}

func TestHostOutputWithActionData(t *testing.T) {
	output := &HostOutput{
		Reply:      "I'll adjust the plan",
		Action:     HostActionAdjust,
		ActionData: map[string]interface{}{"goals": "adjusted goals"},
	}

	data, err := json.Marshal(output)
	require.NoError(t, err)

	var parsed HostOutput
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, HostActionAdjust, parsed.Action)
	assert.NotNil(t, parsed.ActionData)
}
