<div align="center">
<a href="https://stellar.org"><img alt="Stellar" src="https://github.com/stellar/.github/raw/master/stellar-logo.png" width="558" /></a>
<br/>
<strong>Creating equitable access to the global financial system</strong>
<h1>Stellar Horizon API</h1>
</div>
<p align="center">

<a href="https://github.com/stellar/go/actions/workflows/go.yml?query=branch%3Amaster+event%3Apush">![master GitHub workflow](https://github.com/stellar/go/actions/workflows/go.yml/badge.svg)</a>
<a href="https://godoc.org/github.com/stellar/go"><img alt="GoDoc" src="https://godoc.org/github.com/stellar/go?status.svg" /></a>
<a href="https://goreportcard.com/report/github.com/stellar/go"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/stellar/go" /></a>
</p>

This repo is the home for all of the public Go code produced by the [Stellar Development Foundation].

This repo contains various tools and services that you can use and deploy, as well as the SDK you can use to develop applications that integrate with the Stellar network.

## Horizon

Horizon is the client facing API server for the [Stellar ecosystem](https://developers.stellar.org/docs/start/introduction/).  It acts as the interface between [Stellar Core](https://developers.stellar.org/docs/run-core-node/) and applications that want to access the Stellar network. It allows you to submit transactions to the network, check the status of accounts, subscribe to event streams and more.

Check out the following resources to get started:
- [Horizon Development Guide](internal/docs/GUIDE_FOR_DEVELOPERS.md): Instructions for building and developing Horizon. Covers setup, building, testing, and contributing. Also contains some helpful notes and context for Horizon developers.
- [Quickstart Guide](https://github.com/stellar/quickstart): An external tool provided from a separate repository. It builds a docker image which can be used for running the stellar stack including Horizon locally for evaluation and testing situations. A great way to observe a reference runtime deployment, to see how everything fits together.
- [Horizon Testing Guide](internal/docs/TESTING_NOTES.md): Details on how to test Horizon, including unit tests, integration tests, and end-to-end tests.
- [Horizon SDK and API Guide](internal/docs/SDK_API_GUIDE.md): Documentation on the Horizon SDKs, APIs, resources, and examples. Useful for developers building on top of Horizon.

### Run a production server
If you're an administrator planning to run a production instance of Horizon as part of the public Stellar network, you should check out the instructions on our public developer docs - [Run an API Server](https://developers.stellar.org/docs/run-api-server/). It covers installation, monitoring, error scenarios and more.

## Dependencies

This repository is officially supported on the last two releases of Go.

It depends on a [number of external dependencies](./go.mod), and uses Go [Modules](https://github.com/golang/go/wiki/Modules) to manage them. Running any `go` command will automatically download dependencies required for that operation.

You can choose to checkout this repository into a [GOPATH](https://github.com/golang/go/wiki/GOPATH) or into any directory.

### Contributing

As an open source project, development of Horizon is public, and you can help! We welcome new issue reports, documentation and bug fixes, and contributions that further the project roadmap. The [Development Guide](internal/docs/GUIDE_FOR_DEVELOPERS.md) will show you how to build Horizon, see what's going on behind the scenes, and set up an effective develop-test-push cycle so that you can get your work incorporated quickly.


### Developing

See [GUIDE_FOR_DEVELOPERS.md](/internal/docs/GUIDE_FOR_DEVELOPERS.md) for helpful instructions for getting started developing code in this repository.

[Stellar Development Foundation]: https://stellar.org
