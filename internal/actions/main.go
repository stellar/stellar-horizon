package actions

import "github.com/stellar/go/services/horizon/internal/corestate"

type CoreStateGetter interface {
	GetCoreState() corestate.State
}
