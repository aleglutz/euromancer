# euromancer-ssh — build stage
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY euromancer-ssh/go.mod ./
RUN go mod download || true
COPY euromancer-ssh/ ./
RUN go mod tidy && CGO_ENABLED=0 go build -ldflags="-s -w" -o /euromancer-ssh .

# runtime — content baked into the image: push = publish, как и с Pages
FROM alpine:3.21
RUN adduser -D euromancer
USER euromancer
WORKDIR /app
COPY --from=build /euromancer-ssh .
COPY archive/ ./archive/
ENV CONTENT_DIR=/app/archive PORT=23234 HOST_KEY=/data/euromancer_ed25519
EXPOSE 23234
CMD ["./euromancer-ssh"]
