module github.com/nnutter/constable

go 1.27.0

require (
	github.com/stretchr/testify v1.11.1
	golang.org/x/tools v0.49.0
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/alecthomas/go-check-sumtype v0.3.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.uber.org/nilaway v0.0.0-20260808063849-8649a03c818a // indirect
	golang.org/x/exp/typeparams v0.0.0-20260611194520-c48552f49976 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	honnef.co/go/tools v0.8.0 // indirect
)

tool (
	github.com/alecthomas/go-check-sumtype/cmd/go-check-sumtype
	go.uber.org/nilaway/cmd/nilaway
	honnef.co/go/tools/cmd/staticcheck
)
