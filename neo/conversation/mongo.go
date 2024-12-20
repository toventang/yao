package conversation

// Mongo conversation
type Mongo struct{}

// NewMongo create a new conversation
func NewMongo() *Mongo {
	return &Mongo{}
}

// UpdateChatTitle update the chat title
func (conv *Mongo) UpdateChatTitle(sid string, cid string, title string) error {
	return nil
}

// GetChats get the chat list
func (conv *Mongo) GetChats(sid string) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

// GetHistory get the history
func (conv *Mongo) GetHistory(sid string, cid string) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

// SaveHistory save the history
func (conv *Mongo) SaveHistory(sid string, messages []map[string]interface{}, cid string) error {
	return nil
}

// GetRequest get the request
func (conv *Mongo) GetRequest(sid string, rid string) ([]map[string]interface{}, error) {
	return nil, nil
}

// SaveRequest save the request
func (conv *Mongo) SaveRequest(sid string, rid string, cid string, messages []map[string]interface{}) error {
	return nil
}
