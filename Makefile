#!/usr/bin/make -f

AGENT   := svpchain-lending-agent

# GOWORK=off everywhere: a go.work in the parent directory would resolve this
# module's dependencies from sibling checkouts instead of the versions go.mod
# pins, so a build could ship against a revision no tag points at.
GO := GOWORK=off go

.PHONY: build test vet fmt vendor docker deploy clean

build:
	mkdir -p build
	$(GO) build -mod=readonly -o build/$(AGENT) ./cmd/$(AGENT)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w .

# Materialize dependencies so the Docker build is self-contained.
vendor:
	$(GO) mod vendor

docker: vendor
	docker build --platform linux/amd64 \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
		-t ghcr.io/svpchain/$(AGENT):$(VERSION) \
		-f cmd/$(AGENT)/Dockerfile .

deploy:
	./scripts/deploy.sh $(DEPLOY_FLAGS)

clean:
	rm -rf build/ vendor/
