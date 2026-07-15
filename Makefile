@all:
	mkdir -p dist
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	go build -a -ldflags '-extldflags "-static" -s -w' -o dist/psinoza_windows_amd64.exe
	#GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
	#go build -o dist/psinoza_linux_amd64
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
	go build -a -ldflags '-s -w' -o dist/psinoza_darwin_amd64
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
	go build -a -ldflags '-s -w' -o dist/psinoza_darwin_arm64
	lipo -create -output dist/psinoza_darwin_universal \
		dist/psinoza_darwin_amd64 dist/psinoza_darwin_arm64
	rm -f dist/psinoza_darwin_amd64 dist/psinoza_darwin_arm64

@clean:
	rm -rf dist
