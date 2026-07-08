# euromancer-ssh — build stage
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY euromancer-ssh/go.mod ./
RUN go mod download || true
COPY euromancer-ssh/ ./
RUN go mod tidy && CGO_ENABLED=0 go build -ldflags="-s -w" -o /euromancer-ssh .

# tdfiglet — post headlines are rendered at server startup, same as web h1
FROM alpine:3.21 AS tdfiglet
RUN apk add --no-cache build-base git \
 && git clone --depth 1 https://github.com/tat3r/tdfiglet /tmp/tdfiglet \
 && make -C /tmp/tdfiglet \
 && make -C /tmp/tdfiglet install

# runtime — content baked into the image: push = publish, как и с Pages
FROM alpine:3.21
RUN adduser -D euromancer
USER euromancer
WORKDIR /app
COPY --from=build /euromancer-ssh .
COPY --from=tdfiglet /usr/local/bin/tdfiglet /usr/local/bin/tdfiglet
COPY --from=tdfiglet /usr/local/share/tdfiglet /usr/local/share/tdfiglet
COPY archive/ ./archive/
COPY assets/images/ ./assets/images/
ENV CONTENT_DIR=/app/archive PORT=23234 HOST_KEY=/data/euromancer_ed25519
EXPOSE 23234
CMD ["./euromancer-ssh"]
