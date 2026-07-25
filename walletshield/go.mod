module github.com/ZeroKnowledgeNetwork/opt/apps/walletshield

go 1.22

require (
	github.com/BurntSushi/toml v1.5.0
	github.com/fxamacker/cbor/v2 v2.8.0
	github.com/katzenpost/katzenpost v0.0.0-00010101000000-000000000000
	github.com/privacy-ethereum/kps/libs/go v0.2.1
	gopkg.in/op/go-logging.v1 v1.0.0-20160211212156-b2cb9fa56473
)

replace github.com/katzenpost/katzenpost => ../katzenpost
