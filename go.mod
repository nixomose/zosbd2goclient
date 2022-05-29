module github.com/nixomose/zosbd2goclient

replace github.com/nixomose/blockdevicelib => ../blockdevicelib

replace github.com/nixomose/stree_v => ../stree_v

replace github.com/nixomose/zosbd2goclient => ../zosbd2goclient

replace github.com/nixomose/nixomosegotools => ../nixomosegotools

go 1.17

require (
	github.com/nixomose/nixomosegotools v0.0.0-20220322001028-49b7a9e46605
	github.com/nixomose/stree_v v0.0.0-20220326003805-fa1a1f330ccf
)

require (
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ncw/directio v1.0.5 // indirect
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/sys v0.0.0-20220403020550-483a9cbc67c0 // indirect
)
