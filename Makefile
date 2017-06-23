DOCKER_IMAGE ?= elafarge/confsyncer
DOCKER_TAG ?= $(shell git rev-parse --verify --short HEAD)

# (Containerized) build commands
BUILD_CONTAINER = \
  docker run -u $(shell id -u) -it --rm \
	  --workdir "/usr/local/go/src/github.com/elafarge/confsyncer" \
	  -v $${PWD}:/usr/local/go/src/github.com/elafarge/confsyncer:ro \
	  -v $${PWD}/vendor:/vendor/src \
	  -e GOPATH="/go:/vendor" \
	  -e CGO_ENABLED=0 \
	  -e GOOS=linux

GLIDE_CONTAINER = \
	docker run -it --rm \
	  --workdir "/usr/local/go/src/github.com/elafarge/confsyncer" \
	  -v $${PWD}:/usr/local/go/src/github.com/elafarge/confsyncer \
		$(BUILD_CONTAINER_IMAGE)

ETCD3_PUT = export ETCDCTL_API=3; etcdctl --endpoints=http://localhost:20379 put

BUILD_CONTAINER_IMAGE = golang:1-onbuild

COMPOSE_CMD = export USER_ID=$(shell id -u); docker-compose

GOBUILD = go build --installsuffix cgo --ldflags '-extldflags \"-static\"'
GOTEST = go test

.PHONY: default test clean \
	      vendor-update vendor-clean \
	      docker docker-push docker-clean \
				devenv-start devenv-logs devenv-kill populate-etcd

# Default target
default: build/confsyncer ;

# 1. Vendoring
vendor: glide.yaml
	@echo "Pulling dependencies with glide... in a build container"
	rm -rf ./vendor
	mkdir ./vendor
	$(GLIDE_CONTAINER) bash -c \
		"go get github.com/Masterminds/glide && glide install && chown $(shell id -u):$(shell id -g) -R ./glide.lock ./vendor"

vendor-update:
	@echo "Updating dependencies with glide... in a build container"
	rm glide.lock
	$(MAKE) vendor-clean
	$(MAKE) vendor

vendor-clean:
	@echo "Dropping the vendor folder"
	rm -rf ./vendor

# 2. Testing
test:
	@echo "Running go test in $(subst -test,,$(@)) directory"
	$(BUILD_CONTAINER) $(BUILD_CONTAINER_IMAGE) $(GOTEST)

# 3. Compiling
build/confsyncer: *.go main/*.go vendor
	@echo "Building build/confsyncer binary"
	mkdir -p build
	$(BUILD_CONTAINER) -v $${PWD}/build:/build:rw $(BUILD_CONTAINER_IMAGE) \
		$(GOBUILD) -o /build/confsyncer ./main
	chmod 775 ./build/confsyncer

clean:
	@echo "Removing build directory"
	rm -rf build

# 4. Packaging
docker: Dockerfile build/confsyncer # <-- <3 GNU Make <3
	@echo "Building the $(DOCKER_IMAGE):$(DOCKER_TAG) Docker image"
	docker build --build-arg VCS_REF=$(DOCKER_TAG) -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-push:
	@echo "Pushing the $(DOCKER_IMAGE):$(DOCKER_TAG) Docker image"
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

docker-clean:
	@echo "Deleting the $(DOCKER_IMAGE):$(DOCKER_TAG) Docker image"
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) || \
		echo "No $(DOCKER_IMAGE):$(DOCKER_TAG) docker image to remove"

# 5. DevEnv
devenv: build/confsyncer
	@echo "Starting devenv"
	mkdir -p ./data/{conf,etcd}
	$(COMPOSE_CMD) up -d --build

devenv-probe: devenv
	$(COMPOSE_CMD) logs --follow syncer

devenv-tree:
	tree ./data/conf

devenv-down:
	@echo "Destroying devenv"
	$(COMPOSE_CMD) down
	rm -rf ./data

# 5.1: DevEnv fixtures
populate-etcd:
	@echo "Putting sample values in etcd kvstore"
	$(ETCD3_PUT) /myconf/tls/certs_a        vala
	$(ETCD3_PUT) /myconf/tls/certs_b/first  valbfirst
	$(ETCD3_PUT) /myconf/tls/certs_b/second valbsecond
