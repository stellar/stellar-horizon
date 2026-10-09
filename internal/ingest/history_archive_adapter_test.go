package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	stdio "io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/stellar/go-stellar-sdk/historyarchive"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
)

type mockHistoryArchiveAdapter struct {
	mock.Mock
}

func (m *mockHistoryArchiveAdapter) GetLatestLedgerSequence() (uint32, error) {
	args := m.Called()
	return args.Get(0).(uint32), args.Error(1)
}

func (m *mockHistoryArchiveAdapter) BucketListHash(sequence uint32) (xdr.Hash, error) {
	args := m.Called(sequence)
	return args.Get(0).(xdr.Hash), args.Error(1)
}

func (m *mockHistoryArchiveAdapter) GetState(ctx context.Context, sequence uint32) (verifiableChangeReader, error) {
	args := m.Called(ctx, sequence)
	return args.Get(0).(verifiableChangeReader), args.Error(1)
}

func (m *mockHistoryArchiveAdapter) GetStats() []historyarchive.ArchiveStats {
	a := m.Called()
	return a.Get(0).([]historyarchive.ArchiveStats)
}

func TestGetState_Read(t *testing.T) {
	archive, e := getTestArchive()
	if !assert.NoError(t, e) {
		return
	}

	haa := newHistoryArchiveAdapter(archive, network.TestNetworkPassphrase)

	sr, e := haa.GetState(context.Background(), 21686847)
	if !assert.NoError(t, e) {
		return
	}

	lec, e := sr.Read()
	if !assert.NoError(t, e) {
		return
	}
	assert.NotEqual(t, e, stdio.EOF)

	if !assert.NotNil(t, lec) {
		return
	}
	assert.Equal(t, "GAFBQT4VRORLEVEECUYDQGWNVQ563ZN76LGRJR7T7KDL32EES54UOQST", lec.Post.Data.Account.AccountId.Address())
}

func getTestArchive() (historyarchive.ArchiveInterface, error) {
	bucketEntry := xdr.BucketEntry{
		Type: xdr.BucketEntryTypeLiveentry,
		LiveEntry: &xdr.LedgerEntry{
			Data: xdr.LedgerEntryData{
				Type: xdr.LedgerEntryTypeAccount,
				Account: &xdr.AccountEntry{
					AccountId: xdr.MustAddress("GAFBQT4VRORLEVEECUYDQGWNVQ563ZN76LGRJR7T7KDL32EES54UOQST"),
					Balance:   xdr.Int64(200000000),
				},
			},
		},
	}

	// The archive serves bucketEntry as the checkpoint's only bucket. The
	// checkpoint change reader checks every bucket's hash and fails the read
	// on a mismatch, so the HAS lists that bucket's hash and no other bucket.
	var bucket bytes.Buffer
	if err := xdr.MarshalFramed(&bucket, bucketEntry); err != nil {
		return nil, err
	}
	has := historyarchive.HistoryArchiveState{Version: 1, CurrentLedger: 21686847}
	emptyBucket := historyarchive.Hash{}.String()
	for i := range has.CurrentBuckets {
		has.CurrentBuckets[i].Curr = emptyBucket
		has.CurrentBuckets[i].Snap = emptyBucket
	}
	has.CurrentBuckets[0].Curr = historyarchive.Hash(sha256.Sum256(bucket.Bytes())).String()

	mockArchive := &historyarchive.MockArchive{}
	mockArchive.
		On("CategoryCheckpointExists", "history", uint32(21686847)).
		Return(true, nil)
	mockArchive.
		On("GetCheckpointManager").
		Return(historyarchive.NewCheckpointManager(
			historyarchive.DefaultCheckpointFrequency))
	mockArchive.
		On("GetCheckpointHAS", uint32(21686847)).
		Return(has, nil)
	mockArchive.
		On("BucketExists", mock.AnythingOfType("historyarchive.Hash")).
		Return(true, nil)
	mockArchive.
		On("BucketSize", mock.AnythingOfType("historyarchive.Hash")).
		Return(int64(100), nil)
	mockArchive.
		On("GetXdrStreamForHash", mock.AnythingOfType("historyarchive.Hash")).
		Return(xdr.CreateXdrStream(bucketEntry), nil)
	return mockArchive, nil
}
