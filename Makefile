# Makefile — Loom developer convenience targets.
# CI does not depend on this; it calls tools/smoke.sh directly.

.PHONY: build test smoke clean configure help

BUILD ?= build

help:
	@echo "Targets:"
	@echo "  configure   cmake configure into ./$(BUILD)"
	@echo "  build       cmake + go build everything"
	@echo "  test        run all unit tests (ctest + go test)"
	@echo "  smoke       end-to-end smoke test (tools/smoke.sh)"
	@echo "  clean       remove ./$(BUILD) and Go binaries"

configure:
	cmake -S . -B $(BUILD) -DCMAKE_BUILD_TYPE=Debug

build: configure
	cmake --build $(BUILD) -j
	go build -o $(BUILD)/loom-daemon ./daemon/cmd/loom-daemon
	go build -o $(BUILD)/loom         ./cli/cmd/loom

test: build
	cd $(BUILD) && ctest --output-on-failure
	cd cli    && go test ./...
	cd daemon && go test ./...

smoke:
	./tools/smoke.sh

clean:
	rm -rf $(BUILD) cli/loom daemon/loom-daemon
