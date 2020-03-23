PROGRAM=go-chat
ASSETFS_GOFILE=bindata.go

SOURCES=main.go $(ASSETFS_GOFILE)
ASSETS=$(shell find public/ -type f)

$(PROGRAM): $(SOURCES)
	go build -o "$@" $(SOURCES)

$(ASSETFS_GOFILE): $(ASSETS)
	go-bindata-assetfs -o "$@" $^

clean:
	rm -rf $(PROGRAM)
	rm -rf $(ASSETFS_GOFILE)
