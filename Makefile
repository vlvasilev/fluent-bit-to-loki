
REGISTRY                           := hisshadow85
PLUGIN_REPOSITORY         		   := $(REGISTRY)/fluent-bit-to-loki
IMAGE_TAG                          := $(shell cat VERSION)

.PHONY: plugin
plugin:
	go build -buildmode=c-shared -o build/out_loki.so ./cmd

.PHONY: docker-images
docker-images:
	@docker build -t $(PLUGIN_REPOSITORY):$(IMAGE_TAG) -t $(PLUGIN_REPOSITORY):latest -f Dockerfile --target fluent-bit .

.PHONY: revendor
revendor:
	@GO111MODULE=on go mod vendor
	@GO111MODULE=on go mod tidy

.PHONY: check
check:
	@hack/check.sh --golangci-lint-config=./.golangci.yaml ./cmd/...

.PHONY: format
format:
	@./hack/format.sh ./cmd ./extensions ./pkg ./plugin ./test

.PHONY: test
test:
	@./hack/test.sh -r ./cmd/...

.PHONY: verify
verify: check format test