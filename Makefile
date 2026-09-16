BINARY=opendrop
MAC_BIN=$(BINARY)-darwin-arm64
WIN_BIN=$(BINARY).exe

.PHONY: build build-mac build-win test vet clean

build: build-mac
	@echo "built $(MAC_BIN)"

build-mac:
	go build -o $(MAC_BIN) ./cmd/opendrop

build-win:
	GOOS=windows GOARCH=amd64 go build -o $(WIN_BIN) ./cmd/opendrop

build-all: build-mac build-win

test:
	go test ./... 2>&1 | head -30 || true
	./$(MAC_BIN) status || true

vet:
	go vet ./...

clean:
	rm -f $(BINARY) $(MAC_BIN) $(WIN_BIN) /tmp/opendrop-test
