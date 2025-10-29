package history

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/guregu/null"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/xdr"
)

// LedgerBounds represents the ledger bounds of a Stellar transaction
type LedgerBounds struct {
	Null      bool
	MaxLedger null.Int
	MinLedger null.Int
}

// Scan implements the database/sql Scanner interface.
func (t *LedgerBounds) Scan(src interface{}) error {
	if src == nil {
		*t = LedgerBounds{Null: true}
		return nil
	}

	var rangeText string
	switch src := src.(type) {
	case string:
		rangeText = src
	case []byte:
		rangeText = string(src)
	default:
		return errors.Errorf("cannot scan %T", src)
	}

	rangeText = strings.TrimSpace(rangeText)
	if len(rangeText) < 3 {
		return errors.Errorf("range is invalid %s", rangeText)
	}
	inner := rangeText[1 : len(rangeText)-1]
	parts := strings.Split(inner, ",")
	if len(parts) != 2 {
		return errors.Errorf("%s does not have 2 comma separated values", rangeText)
	}

	lower, upper := parts[0], parts[1]
	if len(lower) > 0 {
		if err := t.MinLedger.Scan(lower); err != nil {
			return errors.Wrap(err, "cannot parse lower bound")
		}
	}
	if len(upper) > 0 {
		if err := t.MaxLedger.Scan(upper); err != nil {
			return errors.Wrap(err, "cannot parse upper bound")
		}
	}

	return nil
}

// Value implements the database/sql/driver Valuer interface.
func (t LedgerBounds) Value() (driver.Value, error) {
	if t.Null {
		return nil, nil
	}

	if !t.MaxLedger.Valid || t.MaxLedger.Int64 == 0 {
		return fmt.Sprintf("[%d,)", t.MinLedger.Int64), nil
	}

	return fmt.Sprintf("[%d, %d)", t.MinLedger.Int64, t.MaxLedger.Int64), nil
}

func formatLedgerBounds(ledgerBounds *xdr.LedgerBounds) LedgerBounds {
	if ledgerBounds == nil {
		return LedgerBounds{Null: true}
	}

	return LedgerBounds{
		MinLedger: null.IntFrom(int64(ledgerBounds.MinLedger)),
		// elide max_ledger if it's 0 since that means no upper bound
		MaxLedger: null.NewInt(
			int64(ledgerBounds.MaxLedger),
			ledgerBounds.MaxLedger > 0),
	}
}
