module openlist-ftp-plugin

go 1.25.2

require (
	github.com/OpenListTeam/go-wasi-socket v0.0.0-20251015063839-f7f1b4398e5e
	github.com/OpenListTeam/openlist-wasi-plugin-driver v0.0.0-20251015133414-5b50219c1270
	github.com/jlaffaye/ftp v0.2.1-0.20240918233326-1b970516f5d3
	github.com/jolestar/go-commons-pool/v2 v2.1.2
	golang.org/x/text v0.27.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	go.bytecodealliance.org/cm v0.3.0 // indirect
)

replace github.com/jlaffaye/ftp => ./vendor/ftp
