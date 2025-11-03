FROM golang:1.25 as builder
WORKDIR /build

COPY . .
RUN CGO_ENABLED=0 go build -a -ldflags "-s -w" -o app cmd/app/main.go

FROM alpine:3

RUN apk --no-cache add ca-certificates && update-ca-certificates
RUN addgroup --gid 1000 app
RUN adduser --disabled-password --gecos "" --ingroup app --no-create-home --uid 1000 app

COPY --from=builder /build/app /app

USER 1000
ENTRYPOINT ["/app"]
