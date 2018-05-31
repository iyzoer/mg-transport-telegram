package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func init() {
	config = LoadConfig("config_test.yml")
	orm = NewDb(config)
	logger = newLogger()

	c := Connection{
		ID:       1,
		ClientID: "123123",
		APIKEY:   "test",
		APIURL:   "https://test.retailcrm.ru",
		MGURL:    "https://test.retailcrm.pro",
		MGToken:  "test-token",
		Active:   true,
	}

	c.createConnection()
	orm.DB.Where("token = 123123:Qwerty").Delete(Bot{})
}

func TestRouting_connectHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(connectHandler)

	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code,
		fmt.Sprintf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK))
}

func TestRouting_addBotHandler(t *testing.T) {
	defer gock.Off()

	p := url.Values{"url": {"https://test.com/telegram/123123:Qwerty"}}

	gock.New("https://api.telegram.org").
		Post("/bot123123:Qwerty/getMe").
		Reply(200).
		BodyString(`{"ok":true,"result":{"id":123,"is_bot":true,"first_name":"Test","username":"TestBot"}}`)

	gock.New("https://api.telegram.org").
		Post("/bot123123:Qwerty/setWebhook").
		MatchType("url").
		BodyString(p.Encode()).
		Reply(201).
		BodyString(`{"ok":true}`)

	gock.New("https://api.telegram.org").
		Post("/bot123123:Qwerty/getWebhookInfo").
		Reply(200).
		BodyString(`{"ok":true,"result":{"url":"https://test.com/telegram/123123:Qwerty","has_custom_certificate":false,"pending_update_count":0}}`)

	gock.New("https://test.retailcrm.pro").
		Post("/api/v1/transport/channels").
		BodyString(`{"ID":0,"Type":"telegram","Events":["message_sent","message_updated","message_deleted","message_read"]}`).
		MatchHeader("Content-Type", "application/json").
		MatchHeader("X-Transport-Token", "test-token").
		Reply(201).
		BodyString(`{"id": 1}`)

	req, err := http.NewRequest("POST", "/add-bot/", strings.NewReader(`{"token": "123123:Qwerty", "connectionId": 1}`))
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(addBotHandler)
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code,
		fmt.Sprintf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusCreated))

	bytes, err := ioutil.ReadAll(rr.Body)
	if err != nil {
		t.Fatal(err)
	}

	var res map[string]interface{}

	err = json.Unmarshal(bytes, &res)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "123123:Qwerty", res["token"])
}

func TestRouting_activityBotHandler(t *testing.T) {
	defer gock.Off()

	gock.New("https://test.retailcrm.pro").
		Post("/api/v1/transport/channels").
		BodyString(`{"ID":1,"Type":"telegram","Events":["message_sent","message_updated","message_deleted","message_read"]}`).
		MatchHeader("Content-Type", "application/json").
		MatchHeader("X-Transport-Token", "123123").
		Reply(200).
		BodyString(`{"id": 1}`)

	req, err := http.NewRequest("POST", "/activity-bot/", strings.NewReader(`{"token": "123123:Qwerty", "active": false, "connectionId": 1}`))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(activityBotHandler)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code,
		fmt.Sprintf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK))
}

func TestRouting_settingsHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/settings/123123", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(makeHandler(settingsHandler))
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code,
		fmt.Sprintf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK))
}

func TestRouting_saveHandler(t *testing.T) {
	defer gock.Off()

	gock.New("https://test.retailcrm.ru").
		Get("/api/credentials").
		Reply(200).
		BodyString(`{"success": true, "credentials": ["/api/integration-modules/{code}", "/api/integration-modules/{code}/edit"]}`)

	req, err := http.NewRequest("POST", "/save/",
		strings.NewReader(
			`{"clientId": "123123", 
			"api_url": "https://test.retailcrm.ru",
			"api_key": "test"}`,
		))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(saveHandler)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code,
		fmt.Sprintf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK))
}

func TestRouting_activityHandler(t *testing.T) {
	req, err := http.NewRequest("POST", "/actions/activity",
		strings.NewReader(
			`{"clientId": "123123","activity": {"active": true}}`,
		))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(activityHandler)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code,
		fmt.Sprintf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK))
}

func TestRouting_TranslateLoader(t *testing.T) {
	type m map[string]string
	te := [][]string{}

	dt := "translate"

	files, err := ioutil.ReadDir(dt)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		ms := m{}
		if !f.IsDir() {
			res, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", dt, f.Name()))
			if err != nil {
				t.Fatal(err)
			}

			err = yaml.Unmarshal(res, &ms)
			if err != nil {
				t.Fatal(err)
			}

			keys := []string{}
			for kms := range ms {
				keys = append(keys, kms)
			}
			sort.Strings(keys)
			te = append(te, keys)
		}
	}

}
