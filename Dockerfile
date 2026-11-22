FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN VERSION="$(cat VERSION)" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/custom-gatus-dashboard .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/custom-gatus-dashboard /custom-gatus-dashboard
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/custom-gatus-dashboard"]
