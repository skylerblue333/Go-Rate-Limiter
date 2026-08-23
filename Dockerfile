FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/sky-rate-guard .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sky-rate-guard /sky-rate-guard
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/sky-rate-guard"]
