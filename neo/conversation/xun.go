package conversation

import (
	"fmt"
	"time"

	"github.com/yaoapp/gou/connector"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/xun/capsule"
	"github.com/yaoapp/xun/dbal/query"
	"github.com/yaoapp/xun/dbal/schema"
)

// Xun Database conversation
type Xun struct {
	query   query.Query
	schema  schema.Schema
	setting Setting
}

type row struct {
	Role      string      `json:"role"`
	Title     string      `json:"title"` // Chat title
	Name      string      `json:"name"`  // User name
	Content   string      `json:"content"`
	Sid       string      `json:"sid"`
	Rid       string      `json:"rid"`
	Cid       string      `json:"cid"` // Chat ID from chat history
	ExpiredAt interface{} `json:"expired_at"`
}

// NewXun create a new conversation
func NewXun(setting Setting) (*Xun, error) {

	conv := &Xun{setting: setting}
	if setting.Connector == "default" {
		conv.query = capsule.Global.Query()
		conv.schema = capsule.Global.Schema()

	} else {

		conn, err := connector.Select(setting.Connector)
		if err != nil {
			return nil, err
		}

		conv.query, err = conn.Query()
		if err != nil {
			return nil, err
		}

		conv.schema, err = conn.Schema()
		if err != nil {
			return nil, err
		}
	}

	err := conv.Init()
	if err != nil {
		return nil, err
	}

	return conv, nil
}

// UpdateChatTitle update the chat title
func (conv *Xun) UpdateChatTitle(sid string, cid string, title string) error {
	_, err := conv.query.Table(conv.setting.Table).
		Where("sid", sid).Where("cid", cid).
		Update(map[string]interface{}{"title": title})
	return err
}

// GetChats get the chat list
func (conv *Xun) GetChats(sid string) ([]map[string]interface{}, error) {
	qb := conv.query.Table(conv.setting.Table).
		Select("cid").
		Where("sid", sid).
		GroupBy("cid")

	if conv.setting.TTL > 0 {
		qb.Where("expired_at", ">", time.Now())
	}

	res := []map[string]interface{}{}

	rows, err := qb.Get()
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		res = append(res, map[string]interface{}{
			"chat_id": row.Get("cid"),
			"title":   row.Get("cid"),
		})
	}

	return res, nil
}

// GetHistory get the history
func (conv *Xun) GetHistory(sid string, cid string) ([]map[string]interface{}, error) {

	qb := conv.query.Table(conv.setting.Table).
		Select("role", "name", "content").
		Where("sid", sid).
		Where("cid", cid).
		OrderBy("id", "desc")

	if conv.setting.TTL > 0 {
		qb.Where("expired_at", ">", time.Now())
	}

	limit := 20
	if conv.setting.MaxSize > 0 {
		limit = conv.setting.MaxSize
	}

	rows, err := qb.Limit(limit).Get()
	if err != nil {
		return nil, err
	}

	res := []map[string]interface{}{}
	for _, row := range rows {
		res = append([]map[string]interface{}{{
			"role":    row.Get("role"),
			"name":    row.Get("name"),
			"content": row.Get("content"),
		}}, res...)
	}

	return res, nil
}

// SaveHistory save the history
func (conv *Xun) SaveHistory(sid string, messages []map[string]interface{}, cid string) error {

	defer conv.clean()
	var expiredAt interface{} = nil
	values := []row{}
	if conv.setting.TTL > 0 {
		expiredAt = time.Now().Add(time.Duration(conv.setting.TTL) * time.Second)
	}

	for _, message := range messages {
		value := row{
			Role:      message["role"].(string),
			Name:      "",
			Content:   message["content"].(string),
			Sid:       sid,
			Cid:       cid,
			ExpiredAt: expiredAt,
		}

		if message["name"] != nil {
			value.Name = message["name"].(string)
		}
		values = append(values, value)
	}

	return conv.query.Table(conv.setting.Table).Insert(values)
}

// GetRequest get the request history
func (conv *Xun) GetRequest(sid string, rid string) ([]map[string]interface{}, error) {

	qb := conv.query.Table(conv.setting.Table).
		Select("role", "name", "content", "sid").
		Where("rid", rid).
		Where("sid", sid).
		OrderBy("id", "desc")

	if conv.setting.TTL > 0 {
		qb.Where("expired_at", ">", time.Now())
	}

	limit := 20
	if conv.setting.MaxSize > 0 {
		limit = conv.setting.MaxSize
	}

	rows, err := qb.Limit(limit).Get()
	if err != nil {
		return nil, err
	}

	res := []map[string]interface{}{}
	for _, row := range rows {
		res = append([]map[string]interface{}{{
			"role":    row.Get("role"),
			"name":    row.Get("name"),
			"content": row.Get("content"),
		}}, res...)
	}

	return res, nil
}

// SaveRequest save the request history
func (conv *Xun) SaveRequest(sid string, rid string, cid string, messages []map[string]interface{}) error {

	defer conv.clean()
	var expiredAt interface{} = nil
	values := []row{}
	if conv.setting.TTL > 0 {
		expiredAt = time.Now().Add(time.Duration(conv.setting.TTL) * time.Second)
	}

	for _, message := range messages {
		value := row{
			Role:      message["role"].(string),
			Name:      "",
			Content:   message["content"].(string),
			Sid:       sid,
			Cid:       cid,
			Rid:       rid,
			ExpiredAt: expiredAt,
		}

		if message["name"] != nil {
			value.Name = message["name"].(string)
		}
		values = append(values, value)
	}

	return conv.query.Table(conv.setting.Table).Insert(values)
}

func (conv *Xun) clean() {
	nums, err := conv.query.Table(conv.setting.Table).Where("expired_at", "<=", time.Now()).Delete()
	if err != nil {
		log.Error("Clean the conversation table error: %s", err.Error())
		return
	}

	if nums > 0 {
		log.Trace("Clean the conversation table: %s %d", conv.setting.Table, nums)
	}
}

// Init init the conversation
func (conv *Xun) Init() error {

	has, err := conv.schema.HasTable(conv.setting.Table)
	if err != nil {
		return err
	}

	// create the table
	if !has {
		err = conv.schema.CreateTable(conv.setting.Table, func(table schema.Blueprint) {

			table.ID("id")                            // The ID field
			table.String("sid", 255).Index()          // The Session ID
			table.String("rid", 255).Null().Index()   // The request ID
			table.String("cid", 200).Null().Index()   // The Chat ID
			table.String("role", 200).Null().Index()  // The Message role
			table.String("name", 200).Null().Index()  // The User name
			table.String("title", 200).Null().Index() // The Chat title
			table.Text("content").Null()

			table.TimestampTz("created_at").SetDefaultRaw("NOW()").Index()
			table.TimestampTz("updated_at").Null().Index()
			table.TimestampTz("expired_at").Null().Index()
		})

		if err != nil {
			return err
		}
		log.Trace("Create the conversation table: %s", conv.setting.Table)
	}

	// validate the table
	tab, err := conv.schema.GetTable(conv.setting.Table)
	if err != nil {
		return err
	}

	fields := []string{"id", "sid", "rid", "cid", "role", "name", "content", "created_at", "updated_at", "expired_at"}
	for _, field := range fields {
		if !tab.HasColumn(field) {
			return fmt.Errorf("%s is required", field)
		}
	}

	// Auto update the title
	if !tab.HasColumn("title") {
		err = conv.schema.AlterTable(conv.setting.Table, func(table schema.Blueprint) {
			table.String("title", 200).Null().Index()
		})
		if err != nil {
			return err
		}
	}

	return nil
}
