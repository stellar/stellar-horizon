package actions

import "github.com/stellar/stellar-horizon/internal/corestate"

type CoreStateGetter interface {
	GetCoreState() corestate.State
}
