REGISTRY ?= ghcr.io/gonotelm-lab
TAG ?= dev
GOPROXY ?= https://goproxy.cn,direct
NPM_REGISTRY ?= https://registry.npmmirror.com

.PHONY: run-server run-web web-install docker-build docker-push

run-server:
	@$(MAKE) -C server run-server

run-web:
	@cd web && npm run dev

web-install:
	@cd web && npm install

docker-build:
	docker buildx build --load --build-arg GOPROXY=$(GOPROXY) -t $(REGISTRY)/flow/server:$(TAG) -f server/Dockerfile .
	docker buildx build --load --build-arg NPM_REGISTRY=$(NPM_REGISTRY) -t $(REGISTRY)/flow/web:$(TAG) -f web/Dockerfile web

docker-push:
	docker buildx build --platform linux/amd64,linux/arm64 --push --build-arg GOPROXY=$(GOPROXY) -t $(REGISTRY)/flow/server:$(TAG) -f server/Dockerfile .
	docker buildx build --platform linux/amd64,linux/arm64 --push --build-arg NPM_REGISTRY=$(NPM_REGISTRY) -t $(REGISTRY)/flow/web:$(TAG) -f web/Dockerfile web
