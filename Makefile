PROGRAM=go-chat
ASSETFS_GOFILE=bindata.go

SOURCES=main.go token_bucket.go $(ASSETFS_GOFILE)
ASSETS=$(shell find public/ -type f)

$(PROGRAM): $(SOURCES)
	go build -o "$@" $^

$(ASSETFS_GOFILE): $(ASSETS)
	go-bindata-assetfs -o "$@" $^

clean:
	rm -rf $(PROGRAM)
	rm -rf $(ASSETFS_GOFILE)
