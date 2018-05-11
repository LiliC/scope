build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s" -o critest main.go

image: build
	docker build -t lilic/scope-cri:latest .
	docker push lilic/scope-cri:latest

.PHONY: all build image
