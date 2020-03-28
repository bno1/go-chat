PROGRAM=go-chat
ASSETFS_GOFILE=bindata.go
ASSETS_DIR=public/

SOURCES=main.go token_bucket.go config.go config_parser.go $(ASSETFS_GOFILE)
ASSETS=$(shell find "$(ASSETS_DIR)" -type f)

$(PROGRAM): $(SOURCES)
	go build -o "$@" $^

$(ASSETFS_GOFILE): $(ASSETS)
	go-bindata -fs -prefix "$(ASSETS_DIR)" -o "$@" $^

clean:
	rm -rf $(PROGRAM)
	rm -rf $(ASSETFS_GOFILE)
