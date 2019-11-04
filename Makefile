GIT_TAG:=$(shell git describe --tags)
ifeq ($(GIT_TAG),)
	GIT_TAG:=latest
endif

.PHONY: build
build: 
	gox -os="linux windows" -arch="386 amd64 arm arm64" -output="build/mediacleaner-$(GIT_TAG).{{.OS}}-{{.Arch}}/{{.Dir}}" -verbose ./...
	gox -os="darwin" -arch="386 amd64" -output="build/mediacleaner-$(GIT_TAG).{{.OS}}-{{.Arch}}/{{.Dir}}" -verbose ./...

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
