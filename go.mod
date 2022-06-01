module github.com/nixomose/zosbd2goclient

replace github.com/nixomose/blockdevicelib => ../blockdevicelib

//
replace github.com/nixomose/stree_v => ../stree_v

//
replace github.com/nixomose/zosbd2goclient => ../zosbd2goclient

//
replace github.com/nixomose/nixomosegotools => ../nixomosegotools

go 1.17

require (
	github.com/nixomose/nixomosegotools v0.0.0-20220529231952-c38fcdca5407
	github.com/nixomose/stree_v v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.4.0
)

require (
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ncw/directio v1.0.5 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a // indirect
)
