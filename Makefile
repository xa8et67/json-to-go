TINYGOROOT := $(shell tinygo env TINYGOROOT)

.PHONY: build test linter

build:
	rm -rf static/json2go/wasm_exec.js
	rm -rf static/json2go/main.wasm
	cp $(TINYGOROOT)/targets/wasm_exec.js static/json2go
    # error: could not find wasm-opt, set the WASMOPT environment variable to override。（brew install binaryen fixed it）
	# -no-debug 减少大小  -stack-size=1MB递归调用栈不够
	tinygo build -no-debug -stack-size=1MB -panic=trap -opt=s -size full -o static/json2go/main.wasm -target wasm cmd/wasm/main.go
	tar -zcvf json2go.tar.gz -C static json2go

test:
	go test -v -cover

linter:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.46.2
	golangci-lint cache clean && go clean -modcache -cache -i
	golangci-lint --version
	GOARCH=wasm GOOS=js golangci-lint run --timeout=10m
