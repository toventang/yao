package feishu

import (
	"os"
	"testing"
)

var (
	testAppID     string
	testAppSecret string
)

func TestMain(m *testing.M) {
	testAppID = os.Getenv("FEISHU_TEST_APP_ID")
	testAppSecret = os.Getenv("FEISHU_TEST_APP_SECRET")
	os.Exit(m.Run())
}

func skipIfNoCreds(t *testing.T) {
	t.Helper()
	if testAppID == "" || testAppSecret == "" {
		t.Skip("FEISHU_TEST_APP_ID or FEISHU_TEST_APP_SECRET not set")
	}
}

func testBot() *Bot {
	return NewBot(testAppID, testAppSecret)
}
