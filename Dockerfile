FROM golang:1.11-alpine AS build

# Install tools required for project
# Run `docker build --no-cache .` to update dependencies
RUN apk add --no-cache git
RUN go get -u golang.org/x/crypto/acme/autocert

# List project dependencies with Gopkg.toml and Gopkg.lock
# These layers are only re-built when Gopkg files are updated
# COPY Gopkg.lock Gopkg.toml /go/src/project/
WORKDIR /go/src/project/
# Install library dependencies
# RUN dep ensure -vendor-only

# Copy the entire project and build it
# This layer is rebuilt when a file changes in the project directory
COPY . /go/src/project/
RUN go build -o /bin/project

# This results in a single layer image
FROM frolvlad/alpine-python3:latest
COPY --from=build /bin/project /bin/project

RUN pip3 install --no-cache-dir --upgrade youtube-dl

EXPOSE 8080/tcp

ENTRYPOINT ["/bin/project"]
CMD ["-port", "8080"]