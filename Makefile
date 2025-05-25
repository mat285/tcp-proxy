GIT_SHA ?= $(shell git log --pretty=format:'%H' -n 1 2> /dev/null | cut -c1-8)

.PHONY: deploy
deploy: docker-build apply-manifest


.PHONY: apply-manifest
apply-manifest:
	kubectl apply -f manifest.yml

.PHONY: docker-build
docker-build:
	docker build \
	  --cache-from mat285/ingress-proxy:latest \
	  --platform=linux/amd64 \
	  --tag mat285/ingress-proxy:latest \
	  --tag mat285/ingress-proxy:${GIT_SHA} \
	  --file Dockerfile \
	  --push \
	  .