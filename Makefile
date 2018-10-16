VERSION := $(shell git describe --tags --always --dirty="-dev")
# LDFLAGS := -ldflags='-X "main.version=$(VERSION)"'
Q=@

GOTESTFLAGS = -race
ifndef Q
GOTESTFLAGS += -v
endif

.PHONY: deps
deps:
	$Qdep ensure

.PHONY: clean
clean:
	$Qrm -rf vendor/ && git checkout ./vendor && dep ensure

.PHONY: vet
vet:
	$Qgo vet ./...

.PHONY: fmtcheck
fmtchk:
	$Qexit $(shell goimports -l . | grep -v '^vendor' | wc -l)

.PHONY: fmtfix
fmtfix:
	$Qgoimports -w $(shell find . -iname '*.go' | grep -v vendor)

.PHONY: test
test:
	$Qgo test $(GOTESTFLAGS) -coverpkg="./..." -coverprofile=.coverprofile ./...
	$Qgrep -v types_gen < .coverprofile > .covertmp && mv .covertmp .coverprofile
	$Qgo tool cover -func=.coverprofile

.PHONY: docker
docker:
	$Qdocker build -t jeff-ci:$(VERSION) .

.PHONY: test-ci
test-ci:
	$Qdocker run -v $(shell pwd):/go/src/github.com/abraithwaite/jeff -it jeff-ci:$(VERSION) make deps && make test
