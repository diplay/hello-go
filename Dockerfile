FROM golang:1.11-alpine AS build

# Install tools required for project
# Run `docker build --no-cache .` to update dependencies
RUN apk add --no-cache git
RUN go get -u github.com/go-telegram-bot-api/telegram-bot-api

# List project dependencies with Gopkg.toml and Gopkg.lock
# These layers are only re-built when Gopkg files are updated
# COPY Gopkg.lock Gopkg.toml /go/src/project/
WORKDIR /go/src/hello-go/
# Install library dependencies
# RUN dep ensure -vendor-only

# Copy the entire project and build it
# This layer is rebuilt when a file changes in the project directory
COPY . /go/src/hello-go/
RUN go build -o /bin/project ./cmd/server

# This results in a single layer image
FROM frolvlad/alpine-python3:latest

RUN pip3 install --no-cache-dir --upgrade youtube-dl
RUN apk add --no-cache ffmpeg ca-certificates && update-ca-certificates 2>/dev/null

COPY --from=build /bin/project /bin/project
COPY web/* web/

EXPOSE 8080/tcp

ENTRYPOINT ["/bin/project"]
CMD ["-addr", "0.0.0.0:8080"]
