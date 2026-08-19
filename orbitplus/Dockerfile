FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/orbitplusmaster ./cmd/orbitplusmaster

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/orbitplusmaster /app/orbitplusmaster
COPY --from=build --chown=nonroot:nonroot /src/ui /app/ui
USER nonroot:nonroot
ENTRYPOINT ["/app/orbitplusmaster"]
