FROM golang:1.24-alpine AS base
WORKDIR /app
COPY ./ ./
RUN go build -ldflags="-w -s" -o scrape .

FROM scratch
COPY --from=base /app/scrape ./
ENTRYPOINT [ "./scrape" ]
