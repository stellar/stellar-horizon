<div align="center">
<a href="https://stellar.org"><img alt="Stellar" src="https://github.com/stellar/.github/raw/master/stellar-logo.png" width="558" /></a>
<br/>
<h1>Stellar Horizon API</h1>
</div>
<p align="center">

<a href="https://github.com/stellar/stellar-horizon/actions/workflows/go.yml?query=branch%3Amaster+event%3Apush">![master GitHub workflow](https://github.com/stellar/stellar-horizon/actions/workflows/go.yml/badge.svg)</a>
<a href="https://godoc.org/github.com/stellar/stellar-horizon"><img alt="GoDoc" src="https://godoc.org/github.com/stellar/stellar-horizon?status.svg" /></a>
<a href="https://goreportcard.com/report/github.com/stellar/stellar-horizon"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/stellar/stellar-horizon" /></a>
</p>

Horizon is the client facing API server for the [Stellar ecosystem](https://developers.stellar.org/docs/start/introduction/).  It acts as the interface between [Stellar Core](https://developers.stellar.org/docs/run-core-node/) and applications that want to access the Stellar network. It allows you to submit transactions to the network, check the status of accounts, subscribe to event streams and more.

Check out the following resources to get started:
- [Horizon Development Guide](DEVELOPING.md): Instructions for building and developing Horizon. Covers setup, building, testing, and contributing. Also contains some helpful notes and context for Horizon developers.
- [Quickstart Guide](https://github.com/stellar/quickstart): An external tool provided from a separate repository. It builds a docker image which can be used for running the stellar stack including Horizon locally for evaluation and testing situations. A great way to observe a reference runtime deployment, to see how everything fits together.
- [Horizon Testing Guide](internal/docs/TESTING_NOTES.md): Details on how to test Horizon, including unit tests, integration tests, and end-to-end tests.

### Run a production server
If you're an administrator planning to run a production instance of Horizon as part of the public Stellar network, you should check out the instructions on our public developer docs - [Run an API Server](https://developers.stellar.org/docs/run-api-server/). It covers installation, monitoring, error scenarios and more.

### Dependencies

* Supported on the two latest major Go releases
* Uses [Go Modules](https://github.com/golang/go/wiki/Modules) for dependency management (see [go.mod](./go.mod))
* Standard go build, go test, and go run workflows apply

### Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.

The [Development Guide](DEVELOPING.md) will show you how to build Horizon, see what's going on behind the scenes, and set up an effective develop-test-push cycle so that you can get your work incorporated quickly.

