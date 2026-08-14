FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/orbitplusworker ./cmd/orbitplusworker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/orbitplusworker /orbitplusworker
USER nonroot:nonroot
ENTRYPOINT ["/orbitplusworker"]
