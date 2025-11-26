package horizon

import (
	"log"
	"time"

	"github.com/stellar/throttled"

	"github.com/stellar/go-stellar-sdk/network"
	supportLog "github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/stellar-horizon/internal/test"
)

func NewTestApp(dsn string) *App {
	app, err := NewApp(NewTestConfig(dsn))
	if err != nil {
		log.Fatal("cannot create app", err)
	}
	return app
}

func NewTestConfig(dsn string) Config {
	return Config{
		DatabaseURL: dsn,
		RateQuota: &throttled.RateQuota{
			MaxRate:  throttled.PerHour(1000),
			MaxBurst: 100,
		},
		ConnectionTimeout: 55 * time.Second, // Default
		LogLevel:          supportLog.InfoLevel,
		NetworkPassphrase: network.TestNetworkPassphrase,
	}
}

func NewRequestHelper(app *App) test.RequestHelper {
	return test.NewRequestHelper(app.webServer.Router.Mux)
}
