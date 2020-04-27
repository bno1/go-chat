PROGRAM=go-chat
ASSETFS_GOFILE=generated/bindata.go
ASSETS_DIR=public/
GOFLAGS= # -racer

ASSETS=$(shell find "$(ASSETS_DIR)" -type f)

$(PROGRAM): FORCE $(ASSETFS_GOFILE)
	go build $(GOFLAGS) -o "$@" "./cmd/$@"

$(ASSETFS_GOFILE): $(ASSETS)
	go-bindata -fs -prefix "$(ASSETS_DIR)" -pkg "generated" -o "$@" $^

clean:
	rm -rf $(PROGRAM)
	rm -rf $(ASSETFS_GOFILE)

FORCE:
