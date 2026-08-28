FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy compiled binary (provided by GoReleaser or local build)
COPY flagura /app/flagura

EXPOSE 3000

USER nobody:nobody

ENTRYPOINT ["/app/flagura"]
