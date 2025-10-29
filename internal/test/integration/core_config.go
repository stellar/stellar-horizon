package integration

type validatorCoreConfigTemplatePrams struct {
	Accelerate                            bool
	NetworkPassphrase                     string
	TestingMinimumPersistentEntryLifetime int
	TestingSorobanHighLimitOverride       bool
	OverrideEvictionParamsForTesting      bool
	TestingStartingEvictionScanLevel      uint
	TestingMaxEntriesToArchive            uint
}

type captiveCoreConfigTemplatePrams struct {
	validatorCoreConfigTemplatePrams
	ValidatorAddress string
	PeerPort         int
}

const validatorCoreConfigTemplate = `
ARTIFICIALLY_ACCELERATE_TIME_FOR_TESTING={{ .Accelerate }}

NETWORK_PASSPHRASE="{{ .NetworkPassphrase }}"

TESTING_MINIMUM_PERSISTENT_ENTRY_LIFETIME={{ .TestingMinimumPersistentEntryLifetime }}
TESTING_SOROBAN_HIGH_LIMIT_OVERRIDE={{ .TestingSorobanHighLimitOverride }}

{{if .OverrideEvictionParamsForTesting}}
OVERRIDE_EVICTION_PARAMS_FOR_TESTING=true
TESTING_MAX_ENTRIES_TO_ARCHIVE={{ .TestingMaxEntriesToArchive }}
TESTING_STARTING_EVICTION_SCAN_LEVEL={{ .TestingStartingEvictionScanLevel }}
{{end}}

PEER_PORT=11625
HTTP_PORT=11626
PUBLIC_HTTP_PORT=true

NODE_SEED="SACJC372QBSSKJYTV5A7LWT4NXWHTQO6GHG4QDAVC2XDPX6CNNXFZ4JK"

NODE_IS_VALIDATOR=true
UNSAFE_QUORUM=true
FAILURE_SAFETY=0

BUCKETLIST_DB_INDEX_PAGE_SIZE_EXPONENT = 12
DATABASE = "sqlite3://stellar.db"

[QUORUM_SET]
THRESHOLD_PERCENT=100
VALIDATORS=["GD5KD2KEZJIGTC63IGW6UMUSMVUVG5IHG64HUTFWCHVZH2N2IBOQN7PS"]

[HISTORY.vs]
get="cp history/vs/{0} {1}"
put="cp {0} history/vs/{1}"
mkdir="mkdir -p history/vs/{0}"
`

const captiveCoreConfigTemplate = `
ARTIFICIALLY_ACCELERATE_TIME_FOR_TESTING={{ .Accelerate }}

NETWORK_PASSPHRASE="{{ .NetworkPassphrase }}"

TESTING_MINIMUM_PERSISTENT_ENTRY_LIFETIME={{ .TestingMinimumPersistentEntryLifetime }}
TESTING_SOROBAN_HIGH_LIMIT_OVERRIDE={{ .TestingSorobanHighLimitOverride }}
ENABLE_SOROBAN_DIAGNOSTIC_EVENTS=true

{{if .OverrideEvictionParamsForTesting}}
OVERRIDE_EVICTION_PARAMS_FOR_TESTING=true
TESTING_MAX_ENTRIES_TO_ARCHIVE={{ .TestingMaxEntriesToArchive }}
TESTING_STARTING_EVICTION_SCAN_LEVEL={{ .TestingStartingEvictionScanLevel }}
{{end}}

PEER_PORT={{ .PeerPort }}

UNSAFE_QUORUM=true
FAILURE_SAFETY=0

[[VALIDATORS]]
NAME="local_core"
HOME_DOMAIN="core.local"
# From "SACJC372QBSSKJYTV5A7LWT4NXWHTQO6GHG4QDAVC2XDPX6CNNXFZ4JK"
PUBLIC_KEY="GD5KD2KEZJIGTC63IGW6UMUSMVUVG5IHG64HUTFWCHVZH2N2IBOQN7PS"
ADDRESS="{{ .ValidatorAddress }}"
QUALITY="MEDIUM"
`
