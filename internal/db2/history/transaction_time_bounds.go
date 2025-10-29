package history

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strings"

	"github.com/guregu/null"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/xdr"
)

// TimeBounds represents the time bounds of a Stellar transaction
type TimeBounds struct {
	Null  bool
	Upper null.Int
	Lower null.Int
}

// Scan implements the database/sql Scanner interface.
func (t *TimeBounds) Scan(src interface{}) error {
	if src == nil {
		*t = TimeBounds{Null: true}
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
		if err := t.Lower.Scan(lower); err != nil {
			return errors.Wrap(err, "cannot parse lower bound")
		}
	}
	if len(upper) > 0 {
		if err := t.Upper.Scan(upper); err != nil {
			return errors.Wrap(err, "cannot parse upper bound")
		}
	}

	return nil
}

// Value implements the database/sql/driver Valuer interface.
func (t TimeBounds) Value() (driver.Value, error) {
	if t.Null {
		return nil, nil
	}

	if !t.Upper.Valid {
		return fmt.Sprintf("[%d,)", t.Lower.Int64), nil
	}

	return fmt.Sprintf("[%d, %d)", t.Lower.Int64, t.Upper.Int64), nil
}

func formatTimeBounds(timeBounds *xdr.TimeBounds) TimeBounds {
	if timeBounds == nil {
		return TimeBounds{Null: true}
	}

	if timeBounds.MaxTime == 0 {
		return TimeBounds{
			Lower: null.IntFrom(int64(timeBounds.MinTime)),
		}
	}

	maxTime := timeBounds.MaxTime
	if maxTime > math.MaxInt64 {
		maxTime = math.MaxInt64
	}

	return TimeBounds{
		Lower: null.IntFrom(int64(timeBounds.MinTime)),
		Upper: null.IntFrom(int64(maxTime)),
	}
}
