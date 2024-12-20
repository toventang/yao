package neo

import (
	"context"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/yaoapp/yao/aigc"
	"github.com/yaoapp/yao/neo/conversation"
)

// DSL AI assistant
type DSL struct {
	ID                  string                    `json:"-" yaml:"-"`
	Name                string                    `json:"name,omitempty"`
	Use                 string                    `json:"use,omitempty"`
	Guard               string                    `json:"guard,omitempty"`
	Connector           string                    `json:"connector"`
	ConversationSetting conversation.Setting      `json:"conversation" yaml:"conversation"`
	Option              map[string]interface{}    `json:"option"`
	Prepare             string                    `json:"prepare,omitempty"`
	Write               string                    `json:"write,omitempty"`
	Prompts             []aigc.Prompt             `json:"prompts,omitempty"`
	Allows              []string                  `json:"allows,omitempty"`
	Models              []string                  `json:"models,omitempty"`
	AI                  aigc.AI                   `json:"-" yaml:"-"`
	Conversation        conversation.Conversation `json:"-" yaml:"-"`
	GuardHandlers       []gin.HandlerFunc         `json:"-" yaml:"-"`
}

// Answer the answer interface
type Answer interface {
	Stream(func(w io.Writer) bool) bool
	Status(code int)
	Header(key, value string)
}

// Context the context
type Context struct {
	Sid             string                 `json:"sid" yaml:"-"`
	ChatID          string                 `json:"chat_id,omitempty"`
	Stack           string                 `json:"stack,omitempty"`
	Path            string                 `json:"pathname,omitempty"`
	FormData        map[string]interface{} `json:"formdata,omitempty"`
	Field           *ContextField          `json:"field,omitempty"`
	Namespace       string                 `json:"namespace,omitempty"`
	Config          map[string]interface{} `json:"config,omitempty"`
	Signal          interface{}            `json:"signal,omitempty"`
	context.Context `json:"-" yaml:"-"`
}

// ContextField the context field
type ContextField struct {
	Name string `json:"name,omitempty"`
	Bind string `json:"bind,omitempty"`
}
