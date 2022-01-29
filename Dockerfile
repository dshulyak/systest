FROM golang:alpine3.15
RUN apk add libc6-compat gcc musl-dev
WORKDIR /build/
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod go mod download
RUN --mount=type=cache,target=/root/.cache/go-build go test -v -c -o /bin/tests ./tests/