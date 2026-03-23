package types

import agentcontext "github.com/yaoapp/yao/agent/context"

// HostInput is the unified input format for Host Agent (§5.7)
type HostInput struct {
	Scenario string                 `json:"scenario"` // "assign" | "guide" | "clarify"
	Messages []agentcontext.Message `json:"messages"` // Messages from the human
	Context  *HostContext           `json:"context"`  // Current execution context
}

// HostContext provides execution context to Host Agent.
// Note: Goals is *Goals (struct with Content field), serialized as {"content":"..."}.
// Host Agent prompts must expect this struct format rather than a plain string.
type HostContext struct {
	RobotStatus *RobotStatusSnapshot   `json:"robot_status,omitempty"`
	Goals       *Goals                 `json:"goals,omitempty"`
	Tasks       []Task                 `json:"tasks,omitempty"`
	CurrentTask *Task                  `json:"current_task,omitempty"`
	AgentReply  string                 `json:"agent_reply,omitempty"`
	History     []agentcontext.Message `json:"history,omitempty"`
}

// HostOutput is the structured output from Host Agent
type HostOutput struct {
	Reply       string      `json:"reply"`
	Action      HostAction  `json:"action,omitempty"`
	ActionData  interface{} `json:"action_data,omitempty"`
	WaitForMore bool        `json:"wait_for_more,omitempty"`
}
