.PHONY: build test web native android clean

build:
	bash scripts/build-cli.sh host

test:
	go test -race ./...
	go vet ./...

native:
	bash scripts/build-native.sh host

web:
	bash scripts/build-ui.sh web

android:
	bash scripts/build-ui.sh android

clean:
	rm -rf bin build
