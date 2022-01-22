FROM golang:alpine3.15 AS base
RUN apk add libc6-compat gcc musl-dev
WORKDIR /build/
COPY go.mod go.sum ./
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go mod download

FROM base AS build
COPY . .
ARG module 
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go test -c ./${module}/ -o /build/${module}.test

FROM alpine
ARG module
COPY --from=build /build/${module}.test /bin/${module}