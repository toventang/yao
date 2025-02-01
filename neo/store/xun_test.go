package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaoapp/gou/connector"
	"github.com/yaoapp/xun/capsule"
	"github.com/yaoapp/yao/config"
	"github.com/yaoapp/yao/test"
)

func TestNewXunDefault(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_assistant")

	err := capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	if err != nil {
		t.Fatal(err)
	}

	err = capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")
	if err != nil {
		t.Fatal(err)
	}

	err = capsule.Schema().DropTableIfExists("__unit_test_conversation_assistant")
	if err != nil {
		t.Fatal(err)
	}

	// Add a small delay to ensure table is created
	time.Sleep(100 * time.Millisecond)

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
	})

	if err != nil {
		t.Error(err)
		return
	}

	// Check history table
	has, err := capsule.Schema().HasTable("__unit_test_conversation_history")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, has)

	// Check chat table
	has, err = capsule.Schema().HasTable("__unit_test_conversation_chat")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, has)

	// Check assistant table
	has, err = capsule.Schema().HasTable("__unit_test_conversation_assistant")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, has)

	// Validate table structure by attempting operations
	// Test history operations
	messages := []map[string]interface{}{
		{"role": "user", "content": "test message"},
	}
	err = store.SaveHistory("test_user", messages, "test_chat", nil)
	assert.Nil(t, err)

	history, err := store.GetHistory("test_user", "test_chat")
	assert.Nil(t, err)
	assert.NotEmpty(t, history)

	// Test chat operations
	err = store.UpdateChatTitle("test_user", "test_chat", "Test Chat")
	assert.Nil(t, err)

	chat, err := store.GetChat("test_user", "test_chat")
	assert.Nil(t, err)
	assert.NotNil(t, chat)

	// Test assistant operations
	assistant := map[string]interface{}{
		"name":        "Test Assistant",
		"type":        "assistant",
		"connector":   "test",
		"description": "Test Description",
		"tags":        []string{"test"},
		"mentionable": true,
		"automated":   true,
	}

	id, err := store.SaveAssistant(assistant)
	assert.Nil(t, err)
	assert.NotNil(t, id)

	// Clean up test data
	err = store.DeleteChat("test_user", "test_chat")
	assert.Nil(t, err)

	err = store.DeleteAssistant(id.(string))
	assert.Nil(t, err)
}

func TestNewXunConnector(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()

	conn, err := connector.Select("mysql")
	if err != nil {
		t.Fatal(err)
	}

	sch, err := conn.Schema()
	if err != nil {
		t.Fatal(err)
	}

	defer sch.DropTableIfExists("__unit_test_conversation_history")
	defer sch.DropTableIfExists("__unit_test_conversation_chat")
	defer sch.DropTableIfExists("__unit_test_conversation_assistant")

	sch.DropTableIfExists("__unit_test_conversation_history")
	sch.DropTableIfExists("__unit_test_conversation_chat")
	sch.DropTableIfExists("__unit_test_conversation_assistant")

	// Add a small delay to ensure table is created
	time.Sleep(100 * time.Millisecond)

	store, err := NewXun(Setting{
		Connector: "mysql",
		Prefix:    "__unit_test_conversation_",
	})

	if err != nil {
		t.Error(err)
		return
	}

	// Check history table
	has, err := sch.HasTable("__unit_test_conversation_history")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, has)

	// Check chat table
	has, err = sch.HasTable("__unit_test_conversation_chat")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, has)

	// Check assistant table
	has, err = sch.HasTable("__unit_test_conversation_assistant")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, has)

	// Test basic operations
	messages := []map[string]interface{}{
		{"role": "user", "content": "test message"},
	}
	err = store.SaveHistory("test_user", messages, "test_chat", nil)
	assert.Nil(t, err)

	history, err := store.GetHistory("test_user", "test_chat")
	assert.Nil(t, err)
	assert.NotEmpty(t, history)

	err = store.DeleteChat("test_user", "test_chat")
	assert.Nil(t, err)
}

func TestXunSaveAndGetHistory(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")

	err := capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	if err != nil {
		t.Fatal(err)
	}

	err = capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
		TTL:       3600,
	})

	// save the history
	cid := "123456"
	err = store.SaveHistory("123456", []map[string]interface{}{
		{"role": "user", "name": "user1", "content": "hello"},
		{"role": "assistant", "name": "user1", "content": "Hello there, how"},
	}, cid, nil)
	assert.Nil(t, err)

	// get the history
	data, err := store.GetHistory("123456", cid)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(data))
}

func TestXunSaveAndGetHistoryWithCID(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")

	err := capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	if err != nil {
		t.Fatal(err)
	}

	err = capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
		TTL:       3600,
	})

	// save the history with specific cid
	sid := "123456"
	cid := "789012"
	messages := []map[string]interface{}{
		{"role": "user", "name": "user1", "content": "hello"},
		{"role": "assistant", "name": "assistant1", "content": "Hi! How can I help you?"},
	}
	err = store.SaveHistory(sid, messages, cid, nil)
	assert.Nil(t, err)

	// get the history for specific cid
	data, err := store.GetHistory(sid, cid)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(data))

	// save another message with different cid
	anotherCID := "345678"
	moreMessages := []map[string]interface{}{
		{"role": "user", "name": "user1", "content": "another message"},
	}
	err = store.SaveHistory(sid, moreMessages, anotherCID, nil)
	assert.Nil(t, err)

	// get history for the first cid - should still be 2 messages
	data, err = store.GetHistory(sid, cid)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(data))

	// get history for the second cid - should be 1 message
	data, err = store.GetHistory(sid, anotherCID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(data))

	// get all history for the sid without specifying cid
	allData, err := store.GetHistory(sid, cid)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(allData))
}

func TestXunGetChats(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")

	// Drop both tables before test
	err := capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	if err != nil {
		t.Fatal(err)
	}
	err = capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Save some test chats
	sid := "test_user"
	messages := []map[string]interface{}{
		{"role": "user", "content": "test message"},
	}

	// Create chats with different dates
	for i := 0; i < 5; i++ {
		chatID := fmt.Sprintf("chat_%d", i)
		title := fmt.Sprintf("Test Chat %d", i)

		// Save history first to create the chat
		err = store.SaveHistory(sid, messages, chatID, nil)
		assert.Nil(t, err)

		// Update the chat title
		err = store.UpdateChatTitle(sid, chatID, title)
		assert.Nil(t, err)
	}

	// Test getting chats with default filter
	filter := ChatFilter{
		PageSize: 10,
		Order:    "desc",
	}
	groups, err := store.GetChats(sid, filter)
	if err != nil {
		t.Fatal(err)
	}

	assert.Greater(t, len(groups.Groups), 0)

	// Test with keywords
	filter.Keywords = "test"
	groups, err = store.GetChats(sid, filter)
	if err != nil {
		t.Fatal(err)
	}

	assert.Greater(t, len(groups.Groups), 0)
}

func TestXunDeleteChat(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a test chat
	sid := "test_user"
	cid := "test_chat"
	messages := []map[string]interface{}{
		{"role": "user", "content": "test message"},
	}

	// Save the chat and history
	err = store.SaveHistory(sid, messages, cid, nil)
	assert.Nil(t, err)

	// Verify chat exists
	chat, err := store.GetChat(sid, cid)
	assert.Nil(t, err)
	assert.NotNil(t, chat)

	// Delete the chat
	err = store.DeleteChat(sid, cid)
	assert.Nil(t, err)

	// Verify chat is deleted
	chat, err = store.GetChat(sid, cid)
	assert.Nil(t, err)
	assert.Equal(t, (*ChatInfo)(nil), chat)
}

func TestXunDeleteAllChats(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple test chats
	sid := "test_user"
	messages := []map[string]interface{}{
		{"role": "user", "content": "test message"},
	}

	// Save multiple chats
	for i := 0; i < 3; i++ {
		cid := fmt.Sprintf("test_chat_%d", i)
		err = store.SaveHistory(sid, messages, cid, nil)
		assert.Nil(t, err)
	}

	// Verify chats exist
	response, err := store.GetChats(sid, ChatFilter{})
	assert.Nil(t, err)
	assert.Greater(t, response.Total, int64(0))

	// Delete all chats
	err = store.DeleteAllChats(sid)
	assert.Nil(t, err)

	// Verify all chats are deleted
	response, err = store.GetChats(sid, ChatFilter{})
	assert.Nil(t, err)
	assert.Equal(t, int64(0), response.Total)
}

func TestXunAssistantCRUD(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_assistant")

	// Drop assistant table before test
	err := capsule.Schema().DropTableIfExists("__unit_test_conversation_assistant")
	if err != nil {
		t.Fatal(err)
	}

	// Add a small delay to ensure table is created
	time.Sleep(100 * time.Millisecond)

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Clean up any existing data
	_, err = store.DeleteAssistants(AssistantFilter{})
	assert.Nil(t, err)

	// Test case 1: JSON fields as strings
	tagsJSON := `["tag1", "tag2", "tag3"]`
	optionsJSON := `{"model": "gpt-4"}`
	placeholderJSON := `{"title": "Test Title", "description": "Test Description", "prompts": ["prompt1", "prompt2"]}`
	assistant := map[string]interface{}{
		"name":        "Test Assistant",
		"type":        "assistant",
		"avatar":      "https://example.com/avatar.png",
		"connector":   "openai",
		"description": "Test Description",
		"path":        "/assistants/test",
		"sort":        100,
		"built_in":    true,
		"tags":        tagsJSON,
		"options":     optionsJSON,
		"placeholder": placeholderJSON,
		"mentionable": true,
		"automated":   true,
	}

	// Test SaveAssistant (Create) with string JSON
	v, err := store.SaveAssistant(assistant)
	assert.Nil(t, err)
	assistantID := v.(string)
	assert.NotEmpty(t, assistantID)

	// Test GetAssistant for the first assistant
	assistantData, err := store.GetAssistant(assistantID)
	assert.Nil(t, err)
	assert.NotNil(t, assistantData)
	assert.Equal(t, "Test Assistant", assistantData["name"])
	assert.Equal(t, "assistant", assistantData["type"])
	assert.Equal(t, "https://example.com/avatar.png", assistantData["avatar"])
	assert.Equal(t, "openai", assistantData["connector"])
	assert.Equal(t, "Test Description", assistantData["description"])
	assert.Equal(t, "/assistants/test", assistantData["path"])
	assert.Equal(t, int64(100), assistantData["sort"])
	assert.Equal(t, int64(1), assistantData["built_in"])
	assert.Equal(t, []interface{}{"tag1", "tag2", "tag3"}, assistantData["tags"])
	assert.Equal(t, map[string]interface{}{"model": "gpt-4"}, assistantData["options"])
	assert.Equal(t, map[string]interface{}{
		"title":       "Test Title",
		"description": "Test Description",
		"prompts":     []interface{}{"prompt1", "prompt2"},
	}, assistantData["placeholder"])
	assert.Equal(t, int64(1), assistantData["mentionable"])
	assert.Equal(t, int64(1), assistantData["automated"])

	// Test case 2: JSON fields as native types
	assistant2 := map[string]interface{}{
		"name":        "Test Assistant 2",
		"type":        "assistant",
		"avatar":      "https://example.com/avatar2.png",
		"connector":   "openai",
		"description": "Test Description 2",
		"path":        "/assistants/test2",
		"sort":        200,
		"built_in":    false,
		"tags":        []string{"tag1", "tag2", "tag3"},
		"options":     map[string]interface{}{"model": "gpt-4"},
		"prompts":     []string{"prompt1", "prompt2"},
		"flows":       []string{"flow1", "flow2"},
		"files":       []string{"file1", "file2"},
		"tools":       []map[string]interface{}{{"name": "tool1"}, {"name": "tool2"}},
		"permissions": map[string]interface{}{"read": true, "write": true},
		"placeholder": map[string]interface{}{
			"title":       "Test Title 2",
			"description": "Test Description 2",
			"prompts":     []string{"prompt3", "prompt4"},
		},
		"mentionable": true,
		"automated":   true,
	}

	// Test SaveAssistant (Create) with native types
	v, err = store.SaveAssistant(assistant2)
	assert.Nil(t, err)
	assistant2ID := v.(string)
	assert.NotEmpty(t, assistant2ID)

	// Test case 3: Test with nil JSON fields
	assistant3 := map[string]interface{}{
		"name":        "Test Assistant 3",
		"type":        "assistant",
		"connector":   "openai",
		"description": "Test Description 3",
		"path":        nil,
		"sort":        9999,
		"built_in":    false,
		"tags":        nil,
		"options":     nil,
		"prompts":     nil,
		"flows":       nil,
		"files":       nil,
		"tools":       nil,
		"permissions": nil,
		"placeholder": nil,
		"mentionable": true,
		"automated":   true,
	}

	// Test SaveAssistant (Create) with nil fields
	v, err = store.SaveAssistant(assistant3)
	assert.Nil(t, err)
	assistant3ID := v.(string)
	assert.NotEmpty(t, assistant3ID)

	// Test GetAssistant for the third assistant
	assistant3Data, err := store.GetAssistant(assistant3ID)
	assert.Nil(t, err)
	assert.NotNil(t, assistant3Data)
	assert.Equal(t, "Test Assistant 3", assistant3Data["name"])
	assert.Nil(t, assistant3Data["tags"])
	assert.Nil(t, assistant3Data["options"])
	assert.Nil(t, assistant3Data["prompts"])
	assert.Nil(t, assistant3Data["flows"])
	assert.Nil(t, assistant3Data["files"])
	assert.Nil(t, assistant3Data["tools"])
	assert.Nil(t, assistant3Data["permissions"])
	assert.Nil(t, assistant3Data["placeholder"])
	assert.Equal(t, int64(1), assistant3Data["mentionable"])
	assert.Equal(t, int64(1), assistant3Data["automated"])

	// Test GetAssistant with non-existent ID
	nonExistentData, err := store.GetAssistant("non-existent-id")
	assert.Error(t, err)
	assert.Nil(t, nonExistentData)
	assert.Contains(t, err.Error(), "not found")

	// Test GetAssistants to verify JSON fields are properly stored
	resp, err := store.GetAssistants(AssistantFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 3, len(resp.Data))

	// Clean up all test data
	_, err = store.DeleteAssistants(AssistantFilter{})
	assert.Nil(t, err)

	// Verify cleanup
	resp, err = store.GetAssistants(AssistantFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(resp.Data))
}

func TestXunAssistantPagination(t *testing.T) {
	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_history")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_chat")
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_assistant")

	// Drop assistant table before test
	err := capsule.Schema().DropTableIfExists("__unit_test_conversation_assistant")
	if err != nil {
		t.Fatal(err)
	}

	// Add a small delay to ensure table is created
	time.Sleep(100 * time.Millisecond)

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create test data for filtering tests
	testAssistants := []map[string]interface{}{}
	for i := 0; i < 25; i++ {
		assistant := map[string]interface{}{
			"name":        fmt.Sprintf("Filter Test Assistant %d", i),
			"type":        "assistant",
			"connector":   fmt.Sprintf("connector%d", i%3),
			"description": fmt.Sprintf("Filter Test Description %d", i),
			"tags":        []string{fmt.Sprintf("tag%d", i%5)},
			"built_in":    i%2 == 0,
			"mentionable": i%2 == 0,
			"automated":   i%3 == 0,
			"sort":        9999 - i,
		}
		id, err := store.SaveAssistant(assistant)
		assert.Nil(t, err)
		assistant["assistant_id"] = id
		testAssistants = append(testAssistants, assistant)
	}

	// Get first assistant ID for later use
	firstAssistantID := testAssistants[0]["assistant_id"].(string)

	// Test filtering with assistantIDs
	assistantIDs := []string{firstAssistantID}
	if len(testAssistants) > 1 {
		assistantIDs = append(assistantIDs, testAssistants[1]["assistant_id"].(string))
	}

	// Test multiple assistant_ids
	resp, err := store.GetAssistants(AssistantFilter{
		AssistantIDs: assistantIDs,
		Page:         1,
		PageSize:     10,
	})
	assert.Nil(t, err)
	assert.Equal(t, len(assistantIDs), len(resp.Data))
	for _, assistant := range resp.Data {
		found := false
		for _, id := range assistantIDs {
			if assistant["assistant_id"] == id {
				found = true
				break
			}
		}
		assert.True(t, found, "Assistant ID should be in the requested list")
	}

	// Test assistantIDs with other filters
	resp, err = store.GetAssistants(AssistantFilter{
		AssistantIDs: assistantIDs,
		Select:       []string{"name", "assistant_id", "description"},
		Page:         1,
		PageSize:     10,
	})
	assert.Nil(t, err)
	assert.Equal(t, len(assistantIDs), len(resp.Data))
	// Verify only selected fields are returned
	for _, item := range resp.Data {
		assert.Contains(t, item, "name")
		assert.Contains(t, item, "assistant_id")
		assert.Contains(t, item, "description")
		assert.NotContains(t, item, "tags")
		assert.NotContains(t, item, "options")
	}

	// Test filtering with select fields
	resp, err = store.GetAssistants(AssistantFilter{
		Select:   []string{"name", "description", "tags"},
		Page:     1,
		PageSize: 10,
	})
	assert.Nil(t, err)
	assert.Equal(t, 10, len(resp.Data))

	// Test filtering with select fields and other filters combined
	resp, err = store.GetAssistants(AssistantFilter{
		Tags:     []string{"tag0"},
		Keywords: "Filter Test",
		Select:   []string{"name", "tags"},
		Page:     1,
		PageSize: 10,
	})
	assert.Nil(t, err)
	assert.Greater(t, len(resp.Data), 0)

	// Test combined filters
	mentionableTrue := true
	automatedTrue := true
	resp, err = store.GetAssistants(AssistantFilter{
		Tags:        []string{"tag0"},
		Keywords:    "Filter Test",
		Connector:   "connector0",
		Mentionable: &mentionableTrue,
		Automated:   &automatedTrue,
		Page:        1,
		PageSize:    10,
	})
	assert.Nil(t, err)

	// Now test the delete operations
	// Test delete by connector
	var count int64
	count, err = store.DeleteAssistants(AssistantFilter{
		Connector: "connector0",
	})
	assert.Nil(t, err)
	assert.Greater(t, count, int64(0))

	// Verify deletion
	resp, err = store.GetAssistants(AssistantFilter{
		Connector: "connector0",
	})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(resp.Data))

	// Test delete by built_in status
	builtInTrue := true
	count, err = store.DeleteAssistants(AssistantFilter{
		BuiltIn: &builtInTrue,
	})
	assert.Nil(t, err)
	assert.Greater(t, count, int64(0))

	// Verify deletion
	resp, err = store.GetAssistants(AssistantFilter{
		BuiltIn: &builtInTrue,
	})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(resp.Data))

	// Test delete by tags
	count, err = store.DeleteAssistants(AssistantFilter{
		Tags: []string{"tag1"},
	})
	assert.Nil(t, err)
	assert.Greater(t, count, int64(0))

	// Verify deletion
	resp, err = store.GetAssistants(AssistantFilter{
		Tags: []string{"tag1"},
	})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(resp.Data))

	// Test delete by keywords
	count, err = store.DeleteAssistants(AssistantFilter{
		Keywords: "Filter Test",
	})
	assert.Nil(t, err)
	assert.Greater(t, count, int64(0))

	// Verify all assistants are deleted
	resp, err = store.GetAssistants(AssistantFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(resp.Data))

	// Test delete by assistantIDs
	// First create some test assistants
	testIDs := []string{}
	for i := 0; i < 3; i++ {
		assistant := map[string]interface{}{
			"name":        fmt.Sprintf("AssistantIDs Test Assistant %d", i),
			"type":        "assistant",
			"connector":   "test",
			"description": fmt.Sprintf("AssistantIDs Test Description %d", i),
			"tags":        []string{"test-tag"},
			"built_in":    false,
			"mentionable": true,
			"automated":   true,
		}
		id, err := store.SaveAssistant(assistant)
		assert.Nil(t, err)
		testIDs = append(testIDs, id.(string))
	}

	// Delete by assistantIDs
	count, err = store.DeleteAssistants(AssistantFilter{
		AssistantIDs: testIDs,
	})
	assert.Nil(t, err)
	assert.Equal(t, int64(len(testIDs)), count)

	// Verify deletion
	resp, err = store.GetAssistants(AssistantFilter{
		AssistantIDs: testIDs,
	})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(resp.Data))

	// Verify all assistants are deleted
	resp, err = store.GetAssistants(AssistantFilter{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(resp.Data))
}

func TestGetAssistantTags(t *testing.T) {

	test.Prepare(t, config.Conf)
	defer test.Clean()
	defer capsule.Schema().DropTableIfExists("__unit_test_conversation_assistant")

	store, err := NewXun(Setting{
		Connector: "default",
		Prefix:    "__unit_test_conversation_",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create test assistants with tags
	assistants := []map[string]interface{}{
		{
			"assistant_id": "test-assistant-1",
			"type":         "assistant",
			"connector":    "test",
			"tags":         []string{"tag1", "tag2"},
			"name":         "Test Assistant 1",
		},
		{
			"assistant_id": "test-assistant-2",
			"type":         "assistant",
			"connector":    "test",
			"tags":         []string{"tag2", "tag3"},
			"name":         "Test Assistant 2",
		},
		{
			"assistant_id": "test-assistant-3",
			"type":         "assistant",
			"connector":    "test",
			"tags":         []string{"tag1", "tag3", "tag4"},
			"name":         "Test Assistant 3",
		},
	}

	// Save test assistants
	for _, assistant := range assistants {
		_, err := store.SaveAssistant(assistant)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Get tags
	tags, err := store.GetAssistantTags()
	if err != nil {
		t.Fatal(err)
	}

	// Verify results
	expectedTags := map[string]bool{
		"tag1": true,
		"tag2": true,
		"tag3": true,
		"tag4": true,
	}

	if len(tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(tags))
	}

	for _, tag := range tags {
		if !expectedTags[tag] {
			t.Errorf("Unexpected tag found: %s", tag)
		}
	}

	// Cleanup
	for _, assistant := range assistants {
		err := store.DeleteAssistant(assistant["assistant_id"].(string))
		if err != nil {
			t.Fatal(err)
		}
	}
}
