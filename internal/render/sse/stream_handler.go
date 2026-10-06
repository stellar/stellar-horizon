package sse

import (
	"net/http"
	"time"

	"github.com/stellar/go-stellar-sdk/support/errors"
	"github.com/stellar/stellar-horizon/internal/ledger"
	"github.com/stellar/throttled"
)

// defaultStreamWriteTimeout bounds a single write to a streaming response when
// StreamHandler.WriteTimeout is not set.
const defaultStreamWriteTimeout = 30 * time.Second

type LedgerSourceFactory interface {
	Get() ledger.Source
}

// StreamHandler represents a stream handling action
type StreamHandler struct {
	RateLimiter         *throttled.HTTPRateLimiter
	LedgerSourceFactory LedgerSourceFactory

	// WriteTimeout bounds each individual write to the stream. The handler
	// pushes the deadline forward by this amount before every write, so the
	// server-wide WriteTimeout does not cut off a healthy long-lived stream
	// while a single stalled write is still bounded. It is wired to the
	// connection timeout; when zero, defaultStreamWriteTimeout applies.
	WriteTimeout time.Duration
}

// GenerateEventsFunc generates a slice of sse.Event which are sent via
// streaming.
type GenerateEventsFunc func() ([]Event, error)

// ServeStream handles a SSE requests, sending data every time there is a new
// ledger.
func (handler StreamHandler) ServeStream(
	w http.ResponseWriter,
	r *http.Request,
	limit int,
	generateEvents GenerateEventsFunc,
) {
	ctx := r.Context()
	stream := NewStream(ctx, w)
	stream.SetLimit(limit)

	rc := http.NewResponseController(w)
	writeTimeout := handler.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = defaultStreamWriteTimeout
	}
	// resetWriteDeadline gives the next write a fresh window. Call it right
	// before every write to the stream, so one write's duration is never
	// charged against another's and server work between writes (such as the
	// database query below) does not eat into the write budget. Ignore the
	// error: some response writers (for example httptest.ResponseRecorder in
	// tests) do not support write deadlines, and the stream must still run when
	// they do not.
	resetWriteDeadline := func() {
		_ = rc.SetWriteDeadline(time.Now().Add(writeTimeout))
	}

	ledgerSource := handler.LedgerSourceFactory.Get()
	defer ledgerSource.Close()

	currentLedgerSequence := ledgerSource.CurrentLedger()
	for {
		// Rate limit the request if it's a call to stream since it queries the DB every second. See
		// https://github.com/stellar/go-stellar-sdk/issues/715 for more details.
		rateLimiter := handler.RateLimiter
		if rateLimiter != nil {
			limited, _, err := rateLimiter.RateLimiter.RateLimit(rateLimiter.VaryBy.Key(r), 1)
			if err != nil {
				resetWriteDeadline()
				stream.Err(errors.Wrap(err, "RateLimiter error"))
				return
			}
			if limited {
				resetWriteDeadline()
				stream.Err(ErrRateLimited)
				return
			}
		}

		events, err := generateEvents()
		if err != nil {
			resetWriteDeadline()
			stream.Err(err)
			return
		}
		for _, event := range events {
			if limit <= 0 {
				break
			}
			resetWriteDeadline()
			stream.Send(event)
			limit--
		}

		if limit <= 0 {
			resetWriteDeadline()
			stream.Done()
			return
		}

		// Manually send the preamble in case there are no data events in SSE to trigger a stream.Send call.
		// This method is called every iteration of the loop, but is protected by a sync.Once variable so it's
		// only executed once.
		resetWriteDeadline()
		stream.Init()

		select {
		case currentLedgerSequence = <-ledgerSource.NextLedger(currentLedgerSequence):
			continue
		case <-ctx.Done():
			resetWriteDeadline()
			stream.Done()
			return
		}
	}
}
