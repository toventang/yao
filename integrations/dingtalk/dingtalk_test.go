package dingtalk

import (
	"os"
	"testing"
)

var (
	testClientID     string
	testClientSecret string
)

func TestMain(m *testing.M) {
	testClientID = os.Getenv("DINGTALK_TEST_CLIENT_ID")
	testClientSecret = os.Getenv("DINGTALK_TEST_CLIENT_SECRET")
	os.Exit(m.Run())
}

func skipIfNoCreds(t *testing.T) {
	t.Helper()
	if testClientID == "" || testClientSecret == "" {
		t.Skip("DINGTALK_TEST_CLIENT_ID or DINGTALK_TEST_CLIENT_SECRET not set")
	}
}

func testBotInstance() *Bot {
	return NewBot(testClientID, testClientSecret)
}
