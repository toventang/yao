package assistant

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
	"github.com/yaoapp/gou/fs"
	"github.com/yaoapp/kun/utils"
	chatctx "github.com/yaoapp/yao/neo/context"
	chatMessage "github.com/yaoapp/yao/neo/message"
)

// Get get the assistant by id
func Get(id string) (*Assistant, error) {
	return LoadStore(id)
}

// GetByConnector get the assistant by connector
func GetByConnector(connector string, name string) (*Assistant, error) {
	id := "connector:" + connector

	assistant, exists := loaded.Get(id)
	if exists {
		return assistant, nil
	}

	data := map[string]interface{}{
		"assistant_id": id,
		"connector":    connector,
		"description":  "Default assistant for " + connector,
		"name":         name,
		"type":         "assistant",
	}

	assistant, err := loadMap(data)
	if err != nil {
		return nil, err
	}
	loaded.Put(assistant)
	return assistant, nil
}

// Execute implements the execute functionality
func (ast *Assistant) Execute(c *gin.Context, ctx chatctx.Context, input string, options map[string]interface{}) error {
	contents := chatMessage.NewContents()
	messages, err := ast.withHistory(ctx, input)
	if err != nil {
		return err
	}
	return ast.execute(c, ctx, messages, options, contents)
}

// Execute implements the execute functionality
func (ast *Assistant) execute(c *gin.Context, ctx chatctx.Context, input []chatMessage.Message, userOptions map[string]interface{}, contents *chatMessage.Contents) error {

	if contents == nil {
		contents = chatMessage.NewContents()
	}
	options := ast.withOptions(userOptions)

	// Add RAG and Version support
	ctx.RAG = rag != nil
	ctx.Version = ast.vision

	// Run init hook
	res, err := ast.HookInit(c, ctx, input, options, contents)
	if err != nil {
		chatMessage.New().
			Assistant(ast.ID, ast.Name, ast.Avatar).
			Error(err).
			Done().
			Write(c.Writer)
		return err
	}

	// Update options if provided
	if res != nil && res.Options != nil {
		options = res.Options
	}

	// messages
	if res != nil && res.Input != nil {
		input = res.Input
	}

	// Handle next action
	// It's not used, return the new assistant_id and chat_id
	// if res != nil && res.Next != nil {
	// 	return res.Next.Execute(c, ctx, contents)
	// }

	// Switch to the new assistant if necessary
	if res != nil && res.AssistantID != "" && res.AssistantID != ctx.AssistantID {
		newAst, err := Get(res.AssistantID)
		if err != nil {
			chatMessage.New().
				Assistant(ast.ID, ast.Name, ast.Avatar).
				Error(err).
				Done().
				Write(c.Writer)
			return err
		}

		// Reset Message Contents
		last := input[len(input)-1]
		input, err = newAst.withHistory(ctx, last)
		if err != nil {
			return err
		}

		// Reset options
		options = newAst.withOptions(userOptions)

		// Update options if provided
		if res.Options != nil {
			options = res.Options
		}

		// Update assistant id
		ctx.AssistantID = res.AssistantID
		return newAst.handleChatStream(c, ctx, input, options, contents)
	}

	// Only proceed with chat stream if no specific next action was handled
	return ast.handleChatStream(c, ctx, input, options, contents)
}

// Execute the next action
func (next *NextAction) Execute(c *gin.Context, ctx chatctx.Context, contents *chatMessage.Contents) error {
	switch next.Action {

	// It's not used, because the process could be executed in the hook script
	// It may remove in the future
	// case "process":
	// 	if next.Payload == nil {
	// 		return fmt.Errorf("payload is required")
	// 	}

	// 	name, ok := next.Payload["name"].(string)
	// 	if !ok {
	// 		return fmt.Errorf("process name should be string")
	// 	}

	// 	args := []interface{}{}
	// 	if v, ok := next.Payload["args"].([]interface{}); ok {
	// 		args = v
	// 	}

	// 	// Add context and writer to args
	// 	args = append(args, ctx, c.Writer)
	// 	p, err := process.Of(name, args...)
	// 	if err != nil {
	// 		return fmt.Errorf("get process error: %s", err.Error())
	// 	}

	// 	err = p.Execute()
	// 	if err != nil {
	// 		return fmt.Errorf("execute process error: %s", err.Error())
	// 	}
	// 	defer p.Release()

	// 	return nil

	case "assistant":
		if next.Payload == nil {
			return fmt.Errorf("payload is required")
		}

		// Get assistant id
		id, ok := next.Payload["assistant_id"].(string)
		if !ok {
			return fmt.Errorf("assistant id should be string")
		}

		// Get assistant
		assistant, err := Get(id)
		if err != nil {
			return fmt.Errorf("get assistant error: %s", err.Error())
		}

		// Input
		input := chatMessage.Message{}
		_, has := next.Payload["input"]
		if !has {
			return fmt.Errorf("input is required")
		}

		switch v := next.Payload["input"].(type) {
		case string:
			messages := chatMessage.Message{}
			err := jsoniter.UnmarshalFromString(v, &messages)
			if err != nil {
				return fmt.Errorf("unmarshal input error: %s", err.Error())
			}
			input = messages

		case map[string]interface{}:
			msg, err := chatMessage.NewMap(v)
			if err != nil {
				return fmt.Errorf("unmarshal input error: %s", err.Error())
			}
			input = *msg

		case *chatMessage.Message:
			input = *v

		case chatMessage.Message:
			input = v

		default:
			return fmt.Errorf("input should be string or []chatMessage.Message")
		}

		// Options
		options := map[string]interface{}{}
		if v, ok := next.Payload["options"].(map[string]interface{}); ok {
			options = v
		}

		input.Hidden = true                    // not show in the history
		if input.Name == "" && ctx.Sid != "" { // add user id to the input
			input.Name = ctx.Sid
		}

		messages, err := assistant.withHistory(ctx, input)
		if err != nil {
			return fmt.Errorf("with history error: %s", err.Error())
		}

		// Create a new Text
		// Send loading message and mark as new
		msg := chatMessage.New().Map(map[string]interface{}{
			"new":   true,
			"role":  "assistant",
			"type":  "loading",
			"props": map[string]interface{}{"placeholder": "Calling " + assistant.Name},
		})
		msg.Assistant(assistant.ID, assistant.Name, assistant.Avatar)
		msg.Write(c.Writer)
		newContents := chatMessage.NewContents()

		// Update the context id
		ctx.AssistantID = assistant.ID
		return assistant.execute(c, ctx, messages, options, newContents)

	case "exit":
		return nil

	default:
		return fmt.Errorf("unknown action: %s", next.Action)
	}
}

// GetPlaceholder returns the placeholder of the assistant
func (ast *Assistant) GetPlaceholder() *Placeholder {
	return ast.Placeholder
}

// Call implements the call functionality
func (ast *Assistant) Call(c *gin.Context, payload APIPayload) (interface{}, error) {
	scriptCtx, err := ast.Script.NewContext(payload.Sid, nil)
	if err != nil {
		return nil, err
	}
	defer scriptCtx.Close()
	ctx := c.Request.Context()

	method := fmt.Sprintf("%sAPI", payload.Name)

	// Check if the method exists
	if !scriptCtx.Global().Has(method) {
		return nil, fmt.Errorf(HookErrorMethodNotFound)
	}

	return scriptCtx.CallWith(ctx, method, payload.Payload)
}

// handleChatStream manages the streaming chat interaction with the AI
func (ast *Assistant) handleChatStream(c *gin.Context, ctx chatctx.Context, messages []chatMessage.Message, options map[string]interface{}, contents *chatMessage.Contents) error {
	clientBreak := make(chan bool, 1)
	done := make(chan bool, 1)

	// Chat with AI in background
	go func() {
		err := ast.streamChat(c, ctx, messages, options, clientBreak, done, contents)
		if err != nil {
			chatMessage.New().Error(err).Done().Write(c.Writer)
		}
		done <- true
	}()

	// Wait for completion or client disconnect
	select {
	case <-done:
		return nil
	case <-c.Writer.CloseNotify():
		clientBreak <- true
		return nil
	}
}

// streamChat handles the streaming chat interaction
func (ast *Assistant) streamChat(
	c *gin.Context,
	ctx chatctx.Context,
	messages []chatMessage.Message,
	options map[string]interface{},
	clientBreak chan bool,
	done chan bool,
	contents *chatMessage.Contents) error {

	errorRaw := ""
	isFirst := true
	isFirstThink := true
	isThinking := false

	isFirstTool := true
	isTool := false
	currentMessageID := ""
	err := ast.Chat(c.Request.Context(), messages, options, func(data []byte) int {
		select {
		case <-clientBreak:
			return 0 // break

		default:
			msg := chatMessage.NewOpenAI(data, isThinking)
			if msg == nil {
				return 1 // continue
			}

			if msg.Pending {
				errorRaw += msg.Text
				return 1 // continue
			}

			// Handle error
			if msg.Type == "error" {
				value := msg.String()
				res, hookErr := ast.HookFail(c, ctx, messages, fmt.Errorf("%s", value), contents)
				if hookErr == nil && res != nil && (res.Output != "" || res.Error != "") {
					value = res.Output
					if res.Error != "" {
						value = res.Error
					}
				}
				chatMessage.New().Error(value).Done().Write(c.Writer)
				return 0 // break
			}

			// for api reasoning_content response
			if msg.Type == "think" {
				if isFirstThink {
					msg.Text = "<think>\n" + msg.Text // add the think begin tag
					isFirstThink = false
					isThinking = true
				}
			}

			// for api reasoning_content response
			if isThinking && msg.Type != "think" {
				// add the think close tag
				end := chatMessage.New().Map(map[string]interface{}{"text": "\n</think>\n", "type": "think", "delta": true})
				end.Write(c.Writer)
				end.ID = currentMessageID
				end.AppendTo(contents)
				contents.UpdateType("think", map[string]interface{}{"text": contents.Text()}, currentMessageID)
				isThinking = false

				// Clear the token and make a new line
				contents.NewText([]byte{}, currentMessageID)
				contents.ClearToken()
			}

			// for native tool_calls response
			if msg.Type == "tool_calls_native" {
				if isFirstTool {
					msg.Text = "\n<tool>\n" + msg.Text // add the tool_calls begin tag
					isFirstTool = false
					isTool = true
				}
			}

			// for tool response
			if isTool && msg.Type != "tool_calls_native" {

				if msg.IsDone {
					end := chatMessage.New().Map(map[string]interface{}{"text": "}\n</tool>\n", "type": "tool", "delta": true})
					end.Write(c.Writer)
					end.ID = currentMessageID
					end.AppendTo(contents)
					contents.UpdateType("tool", map[string]interface{}{"text": contents.Text()}, currentMessageID)
					isTool = false
				} else {
					msg.Text = "\n</tool>\n" + msg.Text // add the tool_calls close tag
				}

				isTool = false
			}

			delta := msg.String()

			// Chunk the delta
			if delta != "" {

				msg.AppendTo(contents) // Append content and send message

				// Scan the tokens
				contents.ScanTokens(currentMessageID, func(token string, id string, begin bool, text string, tails string) {
					currentMessageID = id
					msg.ID = id
					msg.Type = token
					msg.Text = ""                                    // clear the text
					msg.Props = map[string]interface{}{"text": text} // Update props

					// End of the token clear the text
					if begin {
						return
					}

					// New message with the tails
					if tails != "" {
						newMsg, err := chatMessage.NewString(tails, id)
						if err != nil {
							return
						}
						messages = append(messages, *newMsg)
					}
				})

				// Handle stream
				// The stream hook is not used, because there's no need to handle the stream output
				// if some thing need to be handled in future, we can use the stream hook again
				// ------------------------------------------------------------------------------
				// res, err := ast.HookStream(c, ctx, messages, msg, contents)
				// if err == nil && res != nil {

				// 	if res.Next != nil {
				// 		err = res.Next.Execute(c, ctx, contents)
				// 		if err != nil {
				// 			chatMessage.New().Error(err.Error()).Done().Write(c.Writer)
				// 		}

				// 		done <- true
				// 		return 0 // break
				// 	}

				// 	if res.Silent {
				// 		return 1 // continue
				// 	}
				// }
				// ------------------------------------------------------------------------------

				// Write the message to the stream
				msgType := msg.Type
				if msgType == "tool_calls_native" {
					msgType = "tool"
				}

				output := chatMessage.New().Map(map[string]interface{}{
					"text":  delta,
					"type":  msgType,
					"done":  msg.IsDone,
					"delta": true,
				})

				if isFirst {
					output.Assistant(ast.ID, ast.Name, ast.Avatar)
					isFirst = false
				}
				output.Write(c.Writer)
			}

			// Complete the stream
			if msg.IsDone {

				// Send the last message to the client
				if delta != "" {
					chatMessage.New().
						Map(map[string]interface{}{
							"assistant_id":     ast.ID,
							"assistant_name":   ast.Name,
							"assistant_avatar": ast.Avatar,
							"text":             delta,
							"type":             "text",
							"delta":            true,
							"done":             true,
						}).
						Write(c.Writer)
				}

				// Remove the last empty data
				contents.RemoveLastEmpty()
				res, hookErr := ast.HookDone(c, ctx, messages, contents)

				// Some error occurred in the hook, return the error
				if hookErr != nil {
					chatMessage.New().Error(hookErr.Error()).Done().Write(c.Writer)
					done <- true
					return 0 // break
				}

				// Save the chat history
				ast.saveChatHistory(ctx, messages, contents)

				// If the hook is successful, execute the next action
				if res != nil && res.Next != nil {
					err := res.Next.Execute(c, ctx, contents)
					if err != nil {
						chatMessage.New().Error(err.Error()).Done().Write(c.Writer)
					}
					done <- true
					return 0 // break
				}

				// The default output
				output := chatMessage.New().Done()
				if res != nil && res.Output != nil {
					output = chatMessage.New().Map(map[string]interface{}{"text": res.Output, "done": true})
				}
				output.Write(c.Writer)
				done <- true
				return 0 // break
			}

			return 1 // continue
		}
	})

	// Handle error
	if err != nil {
		return err
	}

	// raw error
	if errorRaw != "" {
		msg, err := chatMessage.NewStringError(errorRaw)
		if err != nil {
			return fmt.Errorf("error: %s", err.Error())
		}
		msg.Done().Write(c.Writer)
	}

	return nil
}

// saveChatHistory saves the chat history if storage is available
func (ast *Assistant) saveChatHistory(ctx chatctx.Context, messages []chatMessage.Message, contents *chatMessage.Contents) {
	if len(contents.Data) > 0 && ctx.Sid != "" && len(messages) > 0 {
		userMessage := messages[len(messages)-1]
		data := []map[string]interface{}{
			{
				"role":    "user",
				"content": userMessage.Content(),
				"name":    ctx.Sid,
			},
			{
				"role":             "assistant",
				"content":          contents.JSON(),
				"name":             ctx.Sid,
				"assistant_id":     ast.ID,
				"assistant_name":   ast.Name,
				"assistant_avatar": ast.Avatar,
			},
		}

		// if the user message is hidden, just save the assistant message
		if userMessage.Hidden {
			data = []map[string]interface{}{data[1]}
		}

		storage.SaveHistory(ctx.Sid, data, ctx.ChatID, ctx.Map())
	}
}

func (ast *Assistant) withOptions(options map[string]interface{}) map[string]interface{} {
	if options == nil {
		options = map[string]interface{}{}
	}

	// Add Custom Options
	if ast.Options != nil {
		for key, value := range ast.Options {
			options[key] = value
		}
	}

	// Add tool_calls
	if ast.Tools != nil && ast.Tools.Tools != nil && len(ast.Tools.Tools) > 0 {
		if settings, has := connectorSettings[ast.Connector]; has && settings.Tools {
			options["tools"] = ast.Tools.Tools
			if options["tool_choice"] == nil {
				options["tool_choice"] = "auto"
			}
		}
	}

	return options
}

func (ast *Assistant) withPrompts(messages []chatMessage.Message) []chatMessage.Message {
	if ast.Prompts != nil {
		for _, prompt := range ast.Prompts {
			name := ast.Name
			if prompt.Name != "" {
				name = prompt.Name
			}
			messages = append(messages, *chatMessage.New().Map(map[string]interface{}{"role": prompt.Role, "content": prompt.Content, "name": name}))
		}
	}

	// Add tool_calls
	if ast.Tools != nil && ast.Tools.Tools != nil && len(ast.Tools.Tools) > 0 {
		settings, has := connectorSettings[ast.Connector]
		if !has || !settings.Tools {
			raw, _ := jsoniter.MarshalToString(ast.Tools.Tools)
			messages = append(messages, *chatMessage.New().Map(map[string]interface{}{
				"role":    "system",
				"name":    "TOOL_CALLS_SCHEMA",
				"content": raw,
			}))

			messages = append(messages, *chatMessage.New().Map(map[string]interface{}{
				"role": "system",
				"name": "TOOL_CALLS",
				"content": "## Tool Response Format\n" +
					"1. If no matching function exists in TOOL_CALLS_SCHEMA, respond normally without using tool calls\n" +
					"2. When using tools, wrap function calls in <tool> and </tool> tags\n" +
					"3. The tool call must be a valid JSON object\n" +
					"4. Follow the JSON Schema defined in TOOL_CALLS_SCHEMA\n" +
					"5. One complete tool call per response\n" +
					"6. Parameter values MUST strictly follow the descriptions and validation rules defined in properties\n" +
					"7. For each parameter, carefully check and comply with:\n" +
					"   - Data type requirements\n" +
					"   - Format restrictions\n" +
					"   - Value range limitations\n" +
					"   - Pattern matching rules\n" +
					"   - Required field validations\n\n" +
					"Example:\n" +
					"<tool>\n" + `{"function":"<FunctionName>","arguments":{"<ArgumentName>":"<ArgumentValue>"}}` + "\n</tool>",
			}))
			messages = append(messages, *chatMessage.New().Map(map[string]interface{}{
				"role": "system",
				"name": "TOOL_CALLS",
				"content": "## Tool Usage Guidelines\n" +
					"1. Use functions defined in TOOL_CALLS_SCHEMA only when they match your needs\n" +
					"2. If no matching function exists, respond normally as a helpful assistant\n" +
					"3. When using tools, arguments must match the schema definition exactly\n" +
					"4. All parameter values must strictly adhere to the validation rules specified in properties\n" +
					"5. Never skip or ignore any validation requirements defined in the schema",
			}))

			// Add tool_calls prompts
			if ast.Tools.Prompts != nil && len(ast.Tools.Prompts) > 0 {
				for _, prompt := range ast.Tools.Prompts {
					messages = append(messages, *chatMessage.New().Map(map[string]interface{}{
						"role":    prompt.Role,
						"content": prompt.Content,
						"name":    prompt.Name,
					}))
				}
			}
		}
	}

	return messages
}

func (ast *Assistant) withHistory(ctx chatctx.Context, input interface{}) ([]chatMessage.Message, error) {

	var userMessage *chatMessage.Message = chatMessage.New()
	switch v := input.(type) {
	case string:
		userMessage.Map(map[string]interface{}{"role": "user", "content": v})
	case map[string]interface{}:
		userMessage.Map(v)
	case chatMessage.Message:
		userMessage = &v
	case *chatMessage.Message:
		userMessage = v
	default:
		return nil, fmt.Errorf("unknown input type: %T", input)
	}

	messages := []chatMessage.Message{}

	if storage != nil {
		history, err := storage.GetHistory(ctx.Sid, ctx.ChatID)
		if err != nil {
			return nil, err
		}

		// Add history messages
		for _, h := range history {
			msgs, err := chatMessage.NewHistory(h)
			if err != nil {
				return nil, err
			}
			messages = append(messages, msgs...)
		}
	}

	// Add system prompts
	messages = ast.withPrompts(messages)

	// Add user message
	messages = append(messages, *userMessage)
	return messages, nil
}

// Chat implements the chat functionality
func (ast *Assistant) Chat(ctx context.Context, messages []chatMessage.Message, option map[string]interface{}, cb func(data []byte) int) error {
	if ast.openai == nil {
		return fmt.Errorf("openai is not initialized")
	}

	requestMessages, err := ast.requestMessages(ctx, messages)
	if err != nil {
		return fmt.Errorf("request messages error: %s", err.Error())
	}

	_, ext := ast.openai.ChatCompletionsWith(ctx, requestMessages, option, cb)
	if ext != nil {
		return fmt.Errorf("openai chat completions with error: %s", ext.Message)
	}

	return nil
}

func (ast *Assistant) requestMessages(ctx context.Context, messages []chatMessage.Message) ([]map[string]interface{}, error) {

	newMessages := []map[string]interface{}{}
	length := len(messages)

	for index, message := range messages {

		// Ignore the tool, think, error
		if message.Type == "tool" || message.Type == "think" || message.Type == "error" {
			continue
		}

		role := message.Role
		if role == "" {
			return nil, fmt.Errorf("role must be string")
		}

		content := message.String()
		if content == "" {
			return nil, fmt.Errorf("content must be string")
		}

		newMessage := map[string]interface{}{
			"role":    role,
			"content": content,
		}

		// Keep the name for user messages
		if name := message.Name; name != "" {
			if role != "system" {
				newMessage["name"] = stringHash(name)
			} else {
				newMessage["name"] = name
			}
		}

		// Special handling for user messages with JSON content last message
		if role == "user" && index == length-1 {
			content = strings.TrimSpace(content)
			msg, err := chatMessage.NewString(content)
			if err != nil {
				return nil, fmt.Errorf("new string error: %s", err.Error())
			}

			newMessage["content"] = msg.Text
			if message.Attachments != nil {
				contents, err := ast.withAttachments(ctx, &message)
				if err != nil {
					return nil, fmt.Errorf("with attachments error: %s", err.Error())
				}

				// if current assistant is vision capable, add the contents directly
				if ast.vision {
					newMessage["content"] = contents
					continue
				}

				// If current assistant is not vision capable, add the description of the image
				if contents != nil {
					for _, content := range contents {
						newMessages = append(newMessages, content)
					}
				}
			}
		}

		newMessages = append(newMessages, newMessage)
	}

	// For debug environment, print the request messages
	if os.Getenv("YAO_AGENT_PRINT_REQUEST_MESSAGES") == "true" {
		fmt.Println("--- REQUEST_MESSAGES -----------------------------")
		utils.Dump(newMessages)
		fmt.Println("--- END REQUEST_MESSAGES -----------------------------")
	}

	return newMessages, nil
}

func (ast *Assistant) withAttachments(ctx context.Context, msg *chatMessage.Message) ([]map[string]interface{}, error) {
	contents := []map[string]interface{}{{"type": "text", "text": msg.Text}}
	if !ast.vision {
		contents = []map[string]interface{}{{"role": "user", "content": msg.Text}}
	}

	images := []string{}
	for _, attachment := range msg.Attachments {
		if strings.HasPrefix(attachment.ContentType, "image/") {
			if ast.vision {
				images = append(images, attachment.URL)
				continue
			}

			// If the current assistant is not vision capable, add the description of the image
			raw, err := jsoniter.MarshalToString(attachment)
			if err != nil {
				return nil, fmt.Errorf("marshal attachment error: %s", err.Error())
			}
			contents = append(contents, map[string]interface{}{
				"role":    "system",
				"content": raw,
			})
		}
	}

	if len(images) == 0 {
		return contents, nil
	}

	// If the current assistant is vision capable, add the image to the contents directly
	if ast.vision {
		for _, url := range images {

			// If the image is already a URL, add it directly
			if strings.HasPrefix(url, "http") {
				contents = append(contents, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]string{
						"url": url,
					},
				})
				continue
			}

			// Read base64
			bytes64, err := ast.ReadBase64(ctx, url)
			if err != nil {
				return nil, fmt.Errorf("read base64 error: %s", err.Error())
			}
			contents = append(contents, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]string{
					"url": fmt.Sprintf("data:image/jpeg;base64,%s", bytes64),
				},
			})
		}
		return contents, nil
	}

	// If the current assistant is not vision capable, add the description of the image

	return contents, nil
}

// ReadBase64 implements base64 file reading functionality
func (ast *Assistant) ReadBase64(ctx context.Context, fileID string) (string, error) {
	data, err := fs.Get("data")
	if err != nil {
		return "", fmt.Errorf("get filesystem error: %s", err.Error())
	}

	exists, err := data.Exists(fileID)
	if err != nil {
		return "", fmt.Errorf("check file error: %s", err.Error())
	}
	if !exists {
		return "", fmt.Errorf("file %s not found", fileID)
	}

	content, err := data.ReadFile(fileID)
	if err != nil {
		return "", fmt.Errorf("read file error: %s", err.Error())
	}

	return base64.StdEncoding.EncodeToString(content), nil
}
