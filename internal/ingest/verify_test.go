package ingest

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/stellar/go/gxdr"
	ingestsdk "github.com/stellar/go/ingest"
	"github.com/stellar/go/ingest/sac"
	"github.com/stellar/go/randxdr"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/services/horizon/internal/ingest/processors"
	"github.com/stellar/go/services/horizon/internal/test"
	"github.com/stellar/go/support/db"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/xdr"
)

func genAccount(tt *test.T, gen randxdr.Generator) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	numSigners := uint32(rand.Int31n(xdr.MaxSigners))
	gen.Next(
		shape,
		[]randxdr.Preset{
			{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
			{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.ACCOUNT.GetU32())},
			{randxdr.FieldEquals("created.lastModifiedLedgerSeq"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.account.seqNum"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.account.numSubEntries"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.account.balance"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.account.homeDomain"), randxdr.SetPrintableASCII},
			{randxdr.FieldEquals("created.data.account.flags"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.account.signers"), randxdr.SetVecLen(numSigners)},
			{randxdr.FieldMatches(regexp.MustCompile("created\\.data\\.account\\.signers.*weight")), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.account.ext.v1.liabilities.selling"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.account.ext.v1.liabilities.buying"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.account.ext.v1.ext.v2.numSponsoring"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.account.ext.v1.ext.v2.numSponsored"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.account.ext.v1.ext.v2.signerSponsoringIDs"), randxdr.SetVecLen(numSigners)},
			{randxdr.FieldEquals("created.data.account.ext.v1.ext.v2.ext.v3.seqLedger"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.account.ext.v1.ext.v2.ext.v3.seqTime"), randxdr.SetPositiveNum64},
		},
	)

	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genAccountData(tt *test.T, gen randxdr.Generator) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	gen.Next(
		shape,
		[]randxdr.Preset{
			{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
			{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.DATA.GetU32())},
			{randxdr.FieldEquals("created.lastModifiedLedgerSeq"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.data.dataName"), randxdr.SetPrintableASCII},
		},
	)

	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genOffer(tt *test.T, gen randxdr.Generator) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	gen.Next(
		shape,
		[]randxdr.Preset{
			{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
			{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.OFFER.GetU32())},
			{randxdr.FieldEquals("created.lastModifiedLedgerSeq"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.offer.amount"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.offer.price.n"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.offer.price.d"), randxdr.SetPositiveNum32},
		},
	)
	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genLiquidityPool(tt *test.T, gen randxdr.Generator) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	gen.Next(
		shape,
		[]randxdr.Preset{
			{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
			// liquidity pools cannot be sponsored
			{randxdr.FieldEquals("created.ext.v1.sponsoringID"), randxdr.SetPtr(false)},
			{randxdr.FieldEquals("created.lastModifiedLedgerSeq"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.LIQUIDITY_POOL.GetU32())},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.params.fee"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.params.assetA.alphaNum4.assetCode"), randxdr.SetAssetCode},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.params.assetA.alphaNum12.assetCode"), randxdr.SetAssetCode},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.params.assetB.alphaNum4.assetCode"), randxdr.SetAssetCode},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.params.assetB.alphaNum12.assetCode"), randxdr.SetAssetCode},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.reserveA"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.reserveB"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.totalPoolShares"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.liquidityPool.body.constantProduct.poolSharesTrustLineCount"), randxdr.SetPositiveNum64},
		},
	)
	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genTrustLine(tt *test.T, gen randxdr.Generator, extra ...randxdr.Preset) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	presets := []randxdr.Preset{
		{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
		{randxdr.FieldEquals("created.lastModifiedLedgerSeq"), randxdr.SetPositiveNum32},
		{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.TRUSTLINE.GetU32())},
		{randxdr.FieldEquals("created.data.trustLine.flags"), randxdr.SetPositiveNum32},
		{randxdr.FieldEquals("created.data.trustLine.asset.alphaNum4.assetCode"), randxdr.SetAssetCode},
		{randxdr.FieldEquals("created.data.trustLine.asset.alphaNum12.assetCode"), randxdr.SetAssetCode},
		{randxdr.FieldEquals("created.data.trustLine.balance"), randxdr.SetPositiveNum64},
		{randxdr.FieldEquals("created.data.trustLine.limit"), randxdr.SetPositiveNum64},
		{randxdr.FieldEquals("created.data.trustLine.ext.v1.liabilities.selling"), randxdr.SetPositiveNum64},
		{randxdr.FieldEquals("created.data.trustLine.ext.v1.liabilities.buying"), randxdr.SetPositiveNum64},
	}
	presets = append(presets, extra...)
	gen.Next(shape, presets)
	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genClaimableBalance(tt *test.T, gen randxdr.Generator) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	gen.Next(
		shape,
		[]randxdr.Preset{
			{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
			{randxdr.FieldEquals("created.lastModifiedLedgerSeq"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.CLAIMABLE_BALANCE.GetU32())},
			{randxdr.FieldEquals("created.data.claimableBalance.ext.v1.flags"), randxdr.SetPositiveNum32},
			{
				randxdr.And(
					randxdr.IsPtr,
					randxdr.FieldMatches(regexp.MustCompile("created\\.data\\.claimableBalance\\.claimants.*notPredicate")),
				),
				randxdr.SetPtr(true),
			},
			{randxdr.FieldEquals("created.data.claimableBalance.amount"), randxdr.SetPositiveNum64},
			{randxdr.FieldEquals("created.data.claimableBalance.asset.alphaNum4.assetCode"), randxdr.SetAssetCode},
			{randxdr.FieldEquals("created.data.claimableBalance.asset.alphaNum12.assetCode"), randxdr.SetAssetCode},
		},
	)
	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genContractCode(tt *test.T, gen randxdr.Generator) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	gen.Next(
		shape,
		[]randxdr.Preset{
			{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
			{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.CONTRACT_CODE.GetU32())},
		},
	)
	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genTTL(tt *test.T, gen randxdr.Generator) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	gen.Next(
		shape,
		[]randxdr.Preset{
			{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
			{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.TTL.GetU32())},
			{randxdr.FieldEquals("created.lastModifiedLedgerSeq"), randxdr.SetPositiveNum32},
			{randxdr.FieldEquals("created.data.ttl.liveUntilLedgerSeq"), randxdr.SetPositiveNum32},
		},
	)
	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genConfigSetting(tt *test.T, gen randxdr.Generator) xdr.LedgerEntryChange {
	change := xdr.LedgerEntryChange{}
	shape := &gxdr.LedgerEntryChange{}
	gen.Next(
		shape,
		[]randxdr.Preset{
			{randxdr.FieldEquals("type"), randxdr.SetU32(gxdr.LEDGER_ENTRY_CREATED.GetU32())},
			{randxdr.FieldEquals("created.data.type"), randxdr.SetU32(gxdr.CONFIG_SETTING.GetU32())},
		},
	)
	tt.Assert.NoError(gxdr.Convert(shape, &change))
	return change
}

func genAssetContractMetadata(tt *test.T, gen randxdr.Generator) []xdr.LedgerEntryChange {
	assetPreset := randxdr.Preset{
		randxdr.FieldEquals("created.data.trustLine.asset.type"),
		randxdr.SetU32(
			gxdr.ASSET_TYPE_CREDIT_ALPHANUM4.GetU32(),
			gxdr.ASSET_TYPE_CREDIT_ALPHANUM12.GetU32(),
		),
	}
	trustline := genTrustLine(tt, gen, assetPreset)
	assetContractMetadata := assetContractMetadataFromTrustline(tt, trustline)

	otherTrustline := genTrustLine(tt, gen, assetPreset)
	otherAssetContractMetadata := assetContractMetadataFromTrustline(tt, otherTrustline)

	balance := balanceContractDataFromTrustline(tt, trustline)
	otherBalance := balanceContractDataFromTrustline(tt, otherTrustline)
	return []xdr.LedgerEntryChange{
		ttlForContractData(tt, gen, assetContractMetadata),
		assetContractMetadata,
		trustline,
		balance,
		ttlForContractData(tt, gen, balance),
		otherAssetContractMetadata,
		otherBalance,
		ttlForContractData(tt, gen, otherBalance),
		balanceContractDataFromTrustline(tt, genTrustLine(tt, gen, assetPreset)),
		ttlForContractData(tt, gen, otherAssetContractMetadata),
	}
}

func assetContractMetadataFromTrustline(tt *test.T, trustline xdr.LedgerEntryChange) xdr.LedgerEntryChange {
	contractID, err := trustline.Created.Data.MustTrustLine().Asset.ToAsset().ContractID("")
	tt.Assert.NoError(err)
	var assetType xdr.AssetType
	var code, issuer string
	tt.Assert.NoError(
		trustline.Created.Data.MustTrustLine().Asset.Extract(&assetType, &code, &issuer),
	)
	ledgerData, err := sac.AssetToContractData(assetType == xdr.AssetTypeAssetTypeNative, code, issuer, contractID)
	tt.Assert.NoError(err)
	assetContractMetadata := xdr.LedgerEntryChange{
		Type: xdr.LedgerEntryChangeTypeLedgerEntryCreated,
		Created: &xdr.LedgerEntry{
			LastModifiedLedgerSeq: trustline.Created.LastModifiedLedgerSeq,
			Data:                  ledgerData,
		},
	}
	return assetContractMetadata
}

func balanceContractDataFromTrustline(tt *test.T, trustline xdr.LedgerEntryChange) xdr.LedgerEntryChange {
	contractID, err := trustline.Created.Data.MustTrustLine().Asset.ToAsset().ContractID("")
	tt.Assert.NoError(err)
	var assetType xdr.AssetType
	var code, issuer string
	trustlineData := trustline.Created.Data.MustTrustLine()
	tt.Assert.NoError(
		trustlineData.Asset.Extract(&assetType, &code, &issuer),
	)
	assetContractMetadata := xdr.LedgerEntryChange{
		Type: xdr.LedgerEntryChangeTypeLedgerEntryCreated,
		Created: &xdr.LedgerEntry{
			LastModifiedLedgerSeq: trustline.Created.LastModifiedLedgerSeq,
			Data:                  sac.BalanceToContractData(contractID, *trustlineData.AccountId.Ed25519, uint64(trustlineData.Balance)),
		},
	}
	return assetContractMetadata
}

func ttlForContractData(tt *test.T, gen randxdr.Generator, contractData xdr.LedgerEntryChange) xdr.LedgerEntryChange {
	ledgerEntry := contractData.MustCreated()
	lk, err := ledgerEntry.LedgerKey()
	tt.Assert.NoError(err)
	bin, err := lk.MarshalBinary()
	tt.Assert.NoError(err)
	keyHash := sha256.Sum256(bin)
	ttl := genTTL(tt, gen)
	ttl.Created.Data.Ttl.KeyHash = keyHash
	return ttl
}

func assertStateError(t *testing.T, err error, expectStateError bool) {
	_, ok := err.(ingestsdk.StateError)
	if expectStateError {
		assert.True(t, ok, "err should be StateError")
	} else {
		assert.False(t, ok, "err should not be StateError")
	}
}

func makeAccountLedgerEntry() xdr.LedgerEntry {
	entry := xdr.LedgerEntry{
		Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeAccount,
			Account: &xdr.AccountEntry{
				AccountId:  xdr.MustAddress("GC3C4AKRBQLHOJ45U4XG35ESVWRDECWO5XLDGYADO6DPR3L7KIDVUMML"),
				Thresholds: [4]byte{1, 1, 1, 1},
			},
		},
	}
	entry.Normalize()
	return entry
}

func makeOfferLedgerEntry() xdr.LedgerEntry {
	entry := xdr.LedgerEntry{
		Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeOffer,
			Offer: &xdr.OfferEntry{
				SellerId: xdr.MustAddress("GC3C4AKRBQLHOJ45U4XG35ESVWRDECWO5XLDGYADO6DPR3L7KIDVUMML"),
			},
		},
	}
	entry.Normalize()
	return entry
}

func TestStateVerifierTestSuite(t *testing.T) {
	suite.Run(t, new(StateVerifierTestSuite))
}

type StateVerifierTestSuite struct {
	suite.Suite
	verifier        *StateVerifier
	mockStateReader *ingestsdk.MockChangeReader
}

func (s *StateVerifierTestSuite) SetupTest() {
	s.mockStateReader = &ingestsdk.MockChangeReader{}
	s.verifier = NewStateVerifier(s.mockStateReader, nil)
}

func (s *StateVerifierTestSuite) TearDownTest() {
	s.mockStateReader.AssertExpectations(s.T())
}

func (s *StateVerifierTestSuite) TestNoEntries() {
	s.mockStateReader.On("Read").Return(ingestsdk.Change{}, io.EOF).Once()

	entries, err := s.verifier.GetLedgerEntries(10)
	s.Assert().NoError(err)
	s.Assert().Len(entries, 0)
}

func (s *StateVerifierTestSuite) TestReturnErrorOnStateReaderError() {
	s.mockStateReader.On("Read").Return(ingestsdk.Change{}, errors.New("Read error")).Once()

	_, err := s.verifier.GetLedgerEntries(10)
	s.Assert().EqualError(err, "Read error")
}

func (s *StateVerifierTestSuite) TestCurrentEntriesNotEmpty() {
	entry := makeAccountLedgerEntry()
	entryBase64, err := xdr.MarshalBase64(entry)
	s.Assert().NoError(err)

	ledgerKey, err := entry.LedgerKey()
	s.Assert().NoError(err)
	ledgerKeyBase64, err := xdr.MarshalBase64(ledgerKey)
	s.Assert().NoError(err)

	s.verifier.currentEntries = map[string]xdr.LedgerEntry{
		ledgerKeyBase64: entry,
	}

	_, err = s.verifier.GetLedgerEntries(10)
	s.Assert().Error(err)
	assertStateError(s.T(), err, true)
	s.Assert().EqualError(err, "Entries (1) not found locally, example: "+entryBase64)

	err = s.verifier.Verify(10)
	s.Assert().Error(err)
	assertStateError(s.T(), err, true)
	s.Assert().EqualError(err, "Entries (1) not found locally, example: "+entryBase64)
}

func (s *StateVerifierTestSuite) TestTransformFunction() {
	accountEntry := makeAccountLedgerEntry()
	s.mockStateReader.
		On("Read").
		Return(ingestsdk.Change{
			Type: xdr.LedgerEntryTypeAccount,
			Post: &accountEntry,
		}, nil).Once()

	offerEntry := makeOfferLedgerEntry()
	s.mockStateReader.
		On("Read").
		Return(ingestsdk.Change{
			Type: xdr.LedgerEntryTypeOffer,
			Post: &offerEntry,
		}, nil).Once()

	s.mockStateReader.On("Read").Return(ingestsdk.Change{}, io.EOF).Once()

	s.verifier.transformFunction =
		func(entry xdr.LedgerEntry) (ignore bool, newEntry xdr.LedgerEntry) {
			// Leave Account ID only for accounts, ignore the rest
			switch entry.Data.Type {
			case xdr.LedgerEntryTypeAccount:
				accountEntry := entry.Data.Account

				return false, xdr.LedgerEntry{
					Data: xdr.LedgerEntryData{
						Type: xdr.LedgerEntryTypeAccount,
						Account: &xdr.AccountEntry{
							AccountId: accountEntry.AccountId,
						},
					},
				}
			default:
				return true, xdr.LedgerEntry{}
			}
		}

	_, err := s.verifier.GetLedgerEntries(10)
	s.Assert().NoError(err)

	// Check currentEntries
	key, err := accountEntry.LedgerKey()
	s.Assert().NoError(err)
	ledgerKey, err := key.MarshalBinary()
	s.Assert().NoError(err)

	// Account entry transformed and offer entry ignored
	s.Assert().Len(s.verifier.currentEntries, 1)
	s.Assert().Equal(accountEntry, s.verifier.currentEntries[string(ledgerKey)])
}

func (s *StateVerifierTestSuite) TestOnlyRequestedNumberOfKeysReturned() {
	accountEntry := makeAccountLedgerEntry()
	s.mockStateReader.
		On("Read").
		Return(ingestsdk.Change{
			Type: xdr.LedgerEntryTypeAccount,
			Post: &accountEntry,
		}, nil).Once()

	// We don't mock Read() -> (io.Change{}, stdio.EOF) call here
	// because this would execute `stdio.EOF` code path.

	entries, err := s.verifier.GetLedgerEntries(1)
	s.Assert().NoError(err)
	s.Assert().Len(entries, 1)

	// In such case Verify() should notice that not all entries read from buckets
	err = s.verifier.Write(accountEntry)
	s.Assert().NoError(err)

	err = s.verifier.Verify(1)
	s.Assert().Error(err)
	assertStateError(s.T(), err, false)
	s.Assert().EqualError(err, "There are unread entries in state reader. Process all entries before calling Verify.")
}

func (s *StateVerifierTestSuite) TestWriteEntryNotExist() {
	entry := makeAccountLedgerEntry()
	entryBase64, err := xdr.MarshalBase64(entry)
	s.Assert().NoError(err)

	ledgerKey, err := entry.LedgerKey()
	s.Assert().NoError(err)
	ledgerKeyBase64, err := xdr.MarshalBase64(ledgerKey)
	s.Assert().NoError(err)

	err = s.verifier.Write(entry)
	s.Assert().Error(err)
	assertStateError(s.T(), err, true)
	errorMsg := fmt.Sprintf(
		"Cannot find entry in currentEntries map: %s (key = %s)",
		entryBase64,
		ledgerKeyBase64,
	)
	s.Assert().EqualError(err, errorMsg)
}

func (s *StateVerifierTestSuite) TestTransformFunctionBuggyIgnore() {
	accountEntry := makeAccountLedgerEntry()
	s.mockStateReader.
		On("Read").
		Return(ingestsdk.Change{
			Type: xdr.LedgerEntryTypeAccount,
			Post: &accountEntry,
		}, nil).Once()

	s.verifier.transformFunction =
		func(entry xdr.LedgerEntry) (ignore bool, newEntry xdr.LedgerEntry) {
			return false, xdr.LedgerEntry{}
		}

	entries, err := s.verifier.GetLedgerEntries(1)
	s.Assert().NoError(err)
	s.Assert().Len(entries, 1)

	// Check the behavior of transformFunction to code path to test.
	s.verifier.transformFunction =
		func(entry xdr.LedgerEntry) (ignore bool, newEntry xdr.LedgerEntry) {
			return true, xdr.LedgerEntry{}
		}

	entryBase64, err := xdr.MarshalBase64(accountEntry)
	s.Assert().NoError(err)
	errorMsg := fmt.Sprintf(
		"Entry ignored in GetEntries but not ignored in Write: %s. Possibly transformFunction is buggy.",
		entryBase64,
	)
	err = s.verifier.Write(accountEntry)
	s.Assert().EqualError(err, errorMsg)
}

func (s *StateVerifierTestSuite) TestActualExpectedEntryNotEqualWrite() {
	expectedEntry := makeAccountLedgerEntry()
	s.mockStateReader.
		On("Read").
		Return(ingestsdk.Change{
			Type: xdr.LedgerEntryTypeAccount,
			Post: &expectedEntry,
		}, nil).Once()

	entries, err := s.verifier.GetLedgerEntries(1)
	s.Assert().NoError(err)
	s.Assert().Len(entries, 1)

	actualEntry := makeAccountLedgerEntry()
	actualEntry.Data.Account.Thresholds = [4]byte{1, 1, 1, 0}
	actualEntry.Normalize()

	expectedEntryBase64, err := xdr.MarshalBase64(expectedEntry)
	s.Assert().NoError(err)
	actualEntryBase64, err := xdr.MarshalBase64(actualEntry)
	s.Assert().NoError(err)

	errorMsg := fmt.Sprintf(
		"Entry does not match the fetched entry. Expected (history archive): %s (pretransform = %s), actual (horizon): %s",
		expectedEntryBase64,
		expectedEntryBase64,
		actualEntryBase64,
	)
	err = s.verifier.Write(actualEntry)
	s.Assert().Error(err)
	assertStateError(s.T(), err, true)
	s.Assert().EqualError(err, errorMsg)
}

func (s *StateVerifierTestSuite) TestVerifyCountersMatch() {
	accountEntry := makeAccountLedgerEntry()
	s.mockStateReader.
		On("Read").
		Return(ingestsdk.Change{
			Type: xdr.LedgerEntryTypeAccount,
			Post: &accountEntry,
		}, nil).Once()

	s.mockStateReader.On("Read").Return(ingestsdk.Change{}, io.EOF).Once()

	entries, err := s.verifier.GetLedgerEntries(2)
	s.Assert().NoError(err)
	s.Assert().Len(entries, 1)

	err = s.verifier.Write(accountEntry)
	s.Assert().NoError(err)

	err = s.verifier.Verify(10)
	s.Assert().Error(err)
	assertStateError(s.T(), err, true)
	errorMsg := fmt.Sprintf(
		"Number of entries read using GetEntries (%d) does not match number of entries in your storage (%d).",
		1,
		10,
	)
	s.Assert().EqualError(err, errorMsg)

	err = s.verifier.Verify(1)
	s.Assert().NoError(err)
}

func TestTruncateIngestStateTables(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &history.Q{&db.Session{DB: tt.HorizonDB}}

	ledgerEntries := generateRandomLedgerEntries(tt)
	// insert ledger entries of all types into the DB
	tt.Assert.NoError(q.BeginTx(tt.Ctx, &sql.TxOptions{}))
	checkpointLedger := uint32(63)
	changeProcessor := buildChangeProcessor(q, &processors.StatsChangeProcessor{}, historyArchiveSource, checkpointLedger, "")
	for _, change := range ingestsdk.GetChangesFromLedgerEntryChanges(ledgerEntries) {
		tt.Assert.NoError(changeProcessor.ProcessChange(tt.Ctx, change))
	}
	tt.Assert.NoError(changeProcessor.Commit(tt.Ctx))
	tt.Assert.NoError(q.Commit())

	// clear out the state tables
	q.TruncateIngestStateTables(tt.Ctx)

	// reinsert the same ledger entries from before
	tt.Assert.NoError(q.BeginTx(tt.Ctx, &sql.TxOptions{}))
	changeProcessor = buildChangeProcessor(q, &processors.StatsChangeProcessor{}, historyArchiveSource, checkpointLedger, "")
	for _, change := range ingestsdk.GetChangesFromLedgerEntryChanges(ledgerEntries) {
		tt.Assert.NoError(changeProcessor.ProcessChange(tt.Ctx, change))
	}
	// this should succeed if we cleared out the state tables properly
	// otherwise, there will be a duplicate key error when we attempt to
	// insert a row that is already present
	tt.Assert.NoError(changeProcessor.Commit(tt.Ctx))
	tt.Assert.NoError(q.Commit())
}

func TestStateVerifierLockBusy(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &history.Q{&db.Session{DB: tt.HorizonDB}}

	tt.Assert.NoError(q.BeginTx(tt.Ctx, &sql.TxOptions{}))

	checkpointLedger := uint32(63)
	changeProcessor := buildChangeProcessor(q, &processors.StatsChangeProcessor{}, historyArchiveSource, checkpointLedger, "")

	for _, change := range ingestsdk.GetChangesFromLedgerEntryChanges(generateRandomLedgerEntries(tt)) {
		tt.Assert.NoError(changeProcessor.ProcessChange(tt.Ctx, change))
	}
	tt.Assert.NoError(changeProcessor.Commit(tt.Ctx))

	tt.Assert.NoError(q.Commit())

	q.UpdateLastLedgerIngest(tt.Ctx, checkpointLedger)

	mockHistoryAdapter := &mockHistoryArchiveAdapter{}
	sys := &system{
		ctx:                          tt.Ctx,
		historyQ:                     q,
		historyAdapter:               mockHistoryAdapter,
		runStateVerificationOnLedger: ledgerEligibleForStateVerification(64, 1),
		config:                       Config{StateVerificationTimeout: time.Hour},
	}
	sys.initMetrics()

	otherQ := &history.Q{q.Clone()}
	tt.Assert.NoError(otherQ.BeginTx(tt.Ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	}))
	ok, err := otherQ.TryStateVerificationLock(tt.Ctx)
	tt.Assert.NoError(err)
	tt.Assert.True(ok)

	tt.Assert.NoError(sys.verifyState(false, checkpointLedger, xdr.Hash{}))
	mockHistoryAdapter.AssertExpectations(t)

	tt.Assert.NoError(otherQ.Rollback())
}

func TestStateVerifier(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &history.Q{&db.Session{DB: tt.HorizonDB}}

	tt.Assert.NoError(q.BeginTx(tt.Ctx, &sql.TxOptions{}))

	ledger := rand.Int31()
	checkpointLedger := uint32(ledger - (ledger % 64) - 1)
	changeProcessor := buildChangeProcessor(q, &processors.StatsChangeProcessor{}, historyArchiveSource, checkpointLedger, "")
	mockChangeReader := &ingestsdk.MockChangeReader{}

	for _, change := range ingestsdk.GetChangesFromLedgerEntryChanges(generateRandomLedgerEntries(tt)) {
		mockChangeReader.On("Read").Return(change, nil).Once()
		tt.Assert.NoError(changeProcessor.ProcessChange(tt.Ctx, change))
	}
	tt.Assert.NoError(changeProcessor.Commit(tt.Ctx))

	tt.Assert.NoError(q.Commit())

	q.UpdateLastLedgerIngest(tt.Ctx, checkpointLedger)

	mockChangeReader.On("Read").Return(ingestsdk.Change{}, io.EOF).Twice()
	mockChangeReader.On("Close").Return(nil).Once()
	bucketListHash := xdr.Hash{1, 2, 3}
	mockChangeReader.On("VerifyBucketList", bucketListHash).Return(nil).Once()

	mockHistoryAdapter := &mockHistoryArchiveAdapter{}
	mockHistoryAdapter.On("GetState", mock.AnythingOfType("*context.timerCtx"), uint32(checkpointLedger)).Return(mockChangeReader, nil).Once()

	sys := &system{
		ctx:                          tt.Ctx,
		historyQ:                     q,
		historyAdapter:               mockHistoryAdapter,
		runStateVerificationOnLedger: ledgerEligibleForStateVerification(64, 1),
		config:                       Config{StateVerificationTimeout: time.Hour},
	}
	sys.initMetrics()

	tt.Assert.NoError(sys.verifyState(false, checkpointLedger, bucketListHash))
	mockChangeReader.AssertExpectations(t)
	mockHistoryAdapter.AssertExpectations(t)
}

func TestStateVerifierHashError(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &history.Q{&db.Session{DB: tt.HorizonDB}}

	ledger := rand.Int31()
	checkpointLedger := uint32(ledger - (ledger % 64) - 1)
	mockChangeReader := &ingestsdk.MockChangeReader{}

	q.UpdateLastLedgerIngest(tt.Ctx, checkpointLedger)

	mockChangeReader.On("Close").Return(nil).Once()
	bucketListHash := xdr.Hash{1, 2, 3}
	mockChangeReader.On("VerifyBucketList", bucketListHash).Return(fmt.Errorf("hash mismatch error")).Once()

	mockHistoryAdapter := &mockHistoryArchiveAdapter{}
	mockHistoryAdapter.On("GetState", mock.AnythingOfType("*context.timerCtx"), uint32(checkpointLedger)).Return(mockChangeReader, nil).Once()

	sys := &system{
		ctx:                          tt.Ctx,
		historyQ:                     q,
		historyAdapter:               mockHistoryAdapter,
		runStateVerificationOnLedger: ledgerEligibleForStateVerification(64, 1),
		config:                       Config{StateVerificationTimeout: time.Hour},
	}
	sys.initMetrics()

	err := sys.verifyState(false, checkpointLedger, bucketListHash)
	tt.Assert.EqualError(err, "hash mismatch error")
	_, isStateError := err.(ingestsdk.StateError)
	tt.Assert.True(isStateError)
	mockChangeReader.AssertExpectations(t)
	mockHistoryAdapter.AssertExpectations(t)
}

func generateRandomLedgerEntries(tt *test.T) []xdr.LedgerEntryChange {
	gen := randxdr.NewGenerator()

	var changes []xdr.LedgerEntryChange
	for i := 0; i < 100; i++ {
		changes = append(changes,
			genLiquidityPool(tt, gen),
			genClaimableBalance(tt, gen),
			genOffer(tt, gen),
			genTrustLine(tt, gen),
			genAccount(tt, gen),
			genAccountData(tt, gen),
			genContractCode(tt, gen),
			genConfigSetting(tt, gen),
			genTTL(tt, gen),
		)
		changes = append(changes, genAssetContractMetadata(tt, gen)...)
	}

	coverage := map[xdr.LedgerEntryType]int{}
	for _, change := range changes {
		coverage[change.Created.Data.Type]++
	}
	tt.Assert.Equal(len(xdr.LedgerEntryTypeMap), len(coverage))

	return changes
}
