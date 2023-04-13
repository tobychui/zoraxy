# PLATFORMS := darwin/amd64 darwin/arm64 freebsd/amd64 linux/386 linux/amd64 linux/arm linux/arm64 linux/mipsle windows/386 windows/amd64 windows/arm windows/arm64
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm linux/arm64 linux/mipsle  linux/riscv64 windows/amd64 windows/arm64
temp = $(subst /, ,$@)
os = $(word 1, $(temp))
arch = $(word 2, $(temp))

all:  web.tar.gz $(PLATFORMS) fixwindows zoraxy_file_checksum.sha1

binary: $(PLATFORMS)

hash: zoraxy_file_checksum.sha1

web: web.tar.gz

clean: 
	rm -f zoraxy_*_*
	rm -f web.tar.gz

$(PLATFORMS):
	@echo "Building $(os)/$(arch)"
	GOROOT_FINAL=Git/ GOOS=$(os) GOARCH=$(arch) GOARM=6 go build -o './dist/zoraxy_$(os)_$(arch)'  -ldflags "-s -w" -trimpath


fixwindows:
	-mv ./dist/zoraxy_windows_amd64 ./dist/zoraxy_windows_amd64.exe
	-mv ./dist/zoraxy_windows_arm64 ./dist/zoraxy_windows_arm64.exe

web.tar.gz:
	
	@echo "Removing old build resources, if exists"
	-rm -rf ./dist/
	-mkdir ./dist/

	@echo "Moving subfolders to build folder"
	-cp -r ./web ./dist/web/
	-cp -r ./system ./dist/system/

	@ echo "Remove sensitive information"
	-rm -rf ./dist/certs/
	-rm -rf ./dist/conf/
	-rm -rf ./dist/rules/


	@echo "Creating tarball for all required files"
	-rm ./dist/web.tar.gz
	-cd ./dist/ && tar -czf ./web.tar.gz system/ web/

	@echo "Clearing up unzipped folder structures"
	-rm -rf "./dist/web"
	-rm -rf "./dist/system"

zoraxy_file_checksum.sha1:
	@echo "Generating the checksum, if sha1sum installed"
	-sha1sum ./dist/web.tar.gz > ./dist/zoraxy_file_checksum.sha1