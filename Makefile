
GIT_TAG:=$(shell git describe --tags)
VERSION:=$(GIT_TAG)
ifeq ($(GIT_TAG),)
	GIT_TAG:=latest
	VERSION:=$(shell git rev-parse --verify HEAD|awk '{print substr($$0,0,7)}')
endif

OUTPUT_TPL:="build/mediacleaner-$(GIT_TAG).{{.OS}}-{{.Arch}}/{{.Dir}}"

.PHONY: build
build: 
	gox -ldflags "-X github.com/abates/mediacleaner.Version=$(VERSION)" -os="linux windows" -arch="386 amd64 arm arm64" -output="$(OUTPUT_TPL)" -verbose ./...
	gox -ldflags "-X github.com/abates/mediacleaner.Version=$(VERSION)" -os="darwin" -arch="386 amd64" -output="$(OUTPUT_TPL)" -verbose ./...

.PHONY: dist
dist: build
	for dir in `ls build|grep -v tar.gz` ; do tar --strip-components=1 -czvf build/$$dir.tar.gz build/$$dir && rm -rf $$dir ; done

test:
	go test ./...

coverage:
	go test -cover ./... -coverprofile=coverage.out

mediarenamer:
	go build -o build/bin/mediarenamer ./cmd/mediarenamer

mediatranscoder:
	go build -o build/bin/mediatranscoder ./cmd/mediatranscoder

dups:
	go build -o build/bin/dups ./cmd/dups

dtest:
	docker run --rm -v "$(PWD)":/usr/src/project -w /usr/src/project golang:latest go test ./...
