build: dups mediarenamer mediatranscoder

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

