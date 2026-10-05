package actions

import (
	"encoding/json"
	"fmt"
	"net/http"

	hProtocol "github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/support/render/problem"
	horizonContext "github.com/stellar/stellar-horizon/internal/context"
	"github.com/stellar/stellar-horizon/internal/db2/history"
	"github.com/stellar/stellar-horizon/internal/ingest/filters"
)

// these admin HTTP endpoints are documented in internal/httpx/static/admin_oapi.yml
type FilterConfigHandler struct{}

func (handler FilterConfigHandler) GetAssetConfig(w http.ResponseWriter, r *http.Request) {
	historyQ, err := horizonContext.HistoryQFromRequest(r)
	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	config, err := historyQ.GetAssetFilterConfig(r.Context())

	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	responsePayload := handler.assetConfigResource(config)
	enc := json.NewEncoder(w)
	if err = enc.Encode(responsePayload); err != nil {
		problem.Render(r.Context(), w, err)
	}
}

func (handler FilterConfigHandler) GetAccountConfig(w http.ResponseWriter, r *http.Request) {
	historyQ, err := horizonContext.HistoryQFromRequest(r)
	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	config, err := historyQ.GetAccountFilterConfig(r.Context())

	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	responsePayload := handler.accountConfigResource(config)
	enc := json.NewEncoder(w)
	if err = enc.Encode(responsePayload); err != nil {
		problem.Render(r.Context(), w, err)
	}
}

func (handler FilterConfigHandler) UpdateAccountConfig(w http.ResponseWriter, r *http.Request) {
	historyQ, err := horizonContext.HistoryQFromRequest(r)
	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	filterRequest, err := handler.accountFilterResource(r)
	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	filterConfig := history.AccountFilterConfig{}
	filterConfig.Enabled = *filterRequest.Enabled
	filterConfig.Whitelist = filterRequest.Whitelist

	config, err := historyQ.UpdateAccountFilterConfig(r.Context(), filterConfig)
	if err != nil {
		problem.Render(r.Context(), w, err)
	}

	responsePayload := handler.accountConfigResource(config)
	enc := json.NewEncoder(w)
	if err = enc.Encode(responsePayload); err != nil {
		problem.Render(r.Context(), w, err)
	}
}

func (handler FilterConfigHandler) UpdateAssetConfig(w http.ResponseWriter, r *http.Request) {
	historyQ, err := horizonContext.HistoryQFromRequest(r)
	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	filterRequest, err := handler.assetFilterResource(r)
	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	whitelist, err := canonicalAssetWhitelist(filterRequest.Whitelist)
	if err != nil {
		problem.Render(r.Context(), w, err)
		return
	}

	filterConfig := history.AssetFilterConfig{}
	filterConfig.Enabled = *filterRequest.Enabled
	filterConfig.Whitelist = whitelist

	config, err := historyQ.UpdateAssetFilterConfig(r.Context(), filterConfig)
	if err != nil {
		problem.Render(r.Context(), w, err)
	}

	responsePayload := handler.assetConfigResource(config)
	enc := json.NewEncoder(w)
	if err = enc.Encode(responsePayload); err != nil {
		problem.Render(r.Context(), w, err)
	}
}

// canonicalAssetWhitelist parses each whitelist entry as a SEP-11 asset
// ("CODE:ISSUER" or "native") and returns the canonical form the ingestion
// filter compares against. Entries that do not parse are rejected so an entry
// that could never match is not stored.
func canonicalAssetWhitelist(entries []string) ([]string, error) {
	whitelist := make([]string, 0, len(entries))
	for _, entry := range entries {
		asset, err := filters.ParseWhitelistAsset(entry)
		if err != nil {
			return nil, problem.NewProblemWithInvalidField(
				problem.BadRequest,
				"whitelist",
				fmt.Errorf("%q is not a valid asset, expected CODE:ISSUER or native", entry),
			)
		}
		whitelist = append(whitelist, asset.StringCanonical())
	}
	return whitelist, nil
}

func (handler FilterConfigHandler) assetFilterResource(r *http.Request) (hProtocol.AssetFilterConfig, error) {
	var filterRequest hProtocol.AssetFilterConfig
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&filterRequest); err != nil {
		p := problem.NewProblemWithInvalidField(problem.BadRequest, "reason", fmt.Errorf("invalid json for asset filter config %v", err.Error()))
		return hProtocol.AssetFilterConfig{}, p
	}
	return filterRequest, nil
}

func (handler FilterConfigHandler) accountFilterResource(r *http.Request) (hProtocol.AccountFilterConfig, error) {
	var filterRequest hProtocol.AccountFilterConfig
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&filterRequest); err != nil {
		p := problem.NewProblemWithInvalidField(problem.BadRequest, "reason", fmt.Errorf("invalid json for account filter config %v", err.Error()))
		return hProtocol.AccountFilterConfig{}, p
	}
	return filterRequest, nil
}

func (handler FilterConfigHandler) assetConfigResource(config history.AssetFilterConfig) hProtocol.AssetFilterConfig {
	return hProtocol.AssetFilterConfig{
		Whitelist:    config.Whitelist,
		Enabled:      &config.Enabled,
		LastModified: config.LastModified,
	}
}

func (handler FilterConfigHandler) accountConfigResource(config history.AccountFilterConfig) hProtocol.AccountFilterConfig {
	return hProtocol.AccountFilterConfig{
		Whitelist:    config.Whitelist,
		Enabled:      &config.Enabled,
		LastModified: config.LastModified,
	}
}
