package actions

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hProtocol "github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/services/horizon/internal/test"
)

func TestGetAssetFilterConfig(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &history.Q{SessionInterface: tt.HorizonSession()}

	// put some more values into the config for resource validation after retrieval
	fc1 := history.AssetFilterConfig{
		Whitelist: []string{"1", "2"},
		Enabled:   true,
	}

	q.UpdateAssetFilterConfig(tt.Ctx, fc1)

	handler := &FilterConfigHandler{}
	recorder := httptest.NewRecorder()
	handler.GetAssetConfig(
		recorder,
		makeRequest(
			t,
			map[string]string{},
			map[string]string{},
			q,
		),
	)

	resp := recorder.Result()
	tt.Assert.Equal(http.StatusOK, resp.StatusCode)

	raw, err := ioutil.ReadAll(resp.Body)
	tt.Assert.NoError(err)

	var filterCfgResource hProtocol.AssetFilterConfig
	json.Unmarshal(raw, &filterCfgResource)
	tt.Assert.NoError(err)

	tt.Assert.ElementsMatch(filterCfgResource.Whitelist, []string{"1", "2"})
	tt.Assert.Equal(*filterCfgResource.Enabled, true)
	tt.Assert.True(filterCfgResource.LastModified > 0)
}

func TestGetAccountFilterConfig(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &history.Q{SessionInterface: tt.HorizonSession()}

	// put some more values into the config for resource validation after retrieval
	fc1 := history.AccountFilterConfig{
		Whitelist: []string{"1", "2"},
		Enabled:   true,
	}

	q.UpdateAccountFilterConfig(tt.Ctx, fc1)

	handler := &FilterConfigHandler{}
	recorder := httptest.NewRecorder()
	handler.GetAccountConfig(
		recorder,
		makeRequest(
			t,
			map[string]string{},
			map[string]string{},
			q,
		),
	)

	resp := recorder.Result()
	tt.Assert.Equal(http.StatusOK, resp.StatusCode)

	raw, err := ioutil.ReadAll(resp.Body)
	tt.Assert.NoError(err)

	var filterCfgResource hProtocol.AccountFilterConfig
	json.Unmarshal(raw, &filterCfgResource)
	tt.Assert.NoError(err)

	tt.Assert.ElementsMatch(filterCfgResource.Whitelist, []string{"1", "2"})
	tt.Assert.Equal(*filterCfgResource.Enabled, true)
	tt.Assert.True(filterCfgResource.LastModified > 0)
}

func TestMalFormedUpdateAssetFilterConfig(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &history.Q{SessionInterface: tt.HorizonSession()}

	handler := &FilterConfigHandler{}
	recorder := httptest.NewRecorder()
	request := makeRequest(
		t,
		map[string]string{},
		map[string]string{},
		q,
	)

	request.Body = ioutil.NopCloser(strings.NewReader(`
	    {
			"enabled": true
		}
		`))

	handler.UpdateAssetConfig(
		recorder,
		request,
	)

	resp := recorder.Result()
	// can't update a filter when it's missing a required filed, Whitelist
	tt.Assert.Equal(http.StatusBadRequest, resp.StatusCode)
}

func TestMalFormedUpdateAccountFilterConfig(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &history.Q{SessionInterface: tt.HorizonSession()}

	handler := &FilterConfigHandler{}
	recorder := httptest.NewRecorder()
	request := makeRequest(
		t,
		map[string]string{},
		map[string]string{},
		q,
	)

	request.Body = ioutil.NopCloser(strings.NewReader(`
	    {
			"enabled": true
		}
		`))

	handler.UpdateAccountConfig(
		recorder,
		request,
	)

	resp := recorder.Result()
	// can't update a filter when it's missing a required filed, Whitelist
	tt.Assert.Equal(http.StatusBadRequest, resp.StatusCode)
}

func TestUpdateAssetFilterConfig(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &history.Q{SessionInterface: tt.HorizonSession()}

	handler := &FilterConfigHandler{}
	recorder := httptest.NewRecorder()
	request := makeRequest(
		t,
		map[string]string{},
		map[string]string{},
		q,
	)

	request.Body = ioutil.NopCloser(strings.NewReader(`
	    {
			"whitelist": ["4","5","6"],
			"enabled": true
		}`))

	handler.UpdateAssetConfig(
		recorder,
		request,
	)

	resp := recorder.Result()
	tt.Assert.Equal(http.StatusOK, resp.StatusCode)

	raw, err := ioutil.ReadAll(resp.Body)
	tt.Assert.NoError(err)

	var filterCfgResource hProtocol.AssetFilterConfig
	json.Unmarshal(raw, &filterCfgResource)
	tt.Assert.NoError(err)

	tt.Assert.Equal(*filterCfgResource.Enabled, true)
	tt.Assert.True(filterCfgResource.LastModified > 0)
	tt.Assert.ElementsMatch(filterCfgResource.Whitelist, []string{"4", "5", "6"})
}

func TestUpdateAccountFilterConfig(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &history.Q{SessionInterface: tt.HorizonSession()}

	handler := &FilterConfigHandler{}
	recorder := httptest.NewRecorder()
	request := makeRequest(
		t,
		map[string]string{},
		map[string]string{},
		q,
	)

	request.Body = ioutil.NopCloser(strings.NewReader(`
	    {
			"whitelist": ["4","5","6"],
			"enabled": true
		}`))

	handler.UpdateAccountConfig(
		recorder,
		request,
	)

	resp := recorder.Result()
	tt.Assert.Equal(http.StatusOK, resp.StatusCode)

	raw, err := ioutil.ReadAll(resp.Body)
	tt.Assert.NoError(err)

	var filterCfgResource hProtocol.AccountFilterConfig
	json.Unmarshal(raw, &filterCfgResource)
	tt.Assert.NoError(err)

	tt.Assert.Equal(*filterCfgResource.Enabled, true)
	tt.Assert.True(filterCfgResource.LastModified > 0)
	tt.Assert.ElementsMatch(filterCfgResource.Whitelist, []string{"4", "5", "6"})
}
