FROM esacteksab/go:1.25.5-2025-12-12@sha256:0bc9eb9b9f42e0444af62a8d96b8637e9d920bf0d73af7d8375ab0b2337af360 AS builder

ENV GOMODCACHE=/go/pkg/mod

WORKDIR /app

# Copy only module files first to maximize caching
COPY go.mod go.sum ./

# Download modules. This layer will be cached if go.mod/go.sum haven't changed.
# The downloaded files will now be part of this layer's filesystem.
RUN go mod download

# Copy the rest of the application code
COPY . .

# Keep cache mounts here for build performance (Go build cache + reusing modules during build)
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build scripts/build-dev.sh
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build scripts/help-docker.sh

# --- Test Stage ---
FROM builder AS test-stage

RUN mkdir -p /app/coverdata
ENV GOCOVERDIR=/app/coverdata

# !!! DO NOT ADD A `-v` TO THIS CMD AND ALLOW TO RUN IN GITHUB ACTIONS. YOU **WILL** LEAK GITHUB_TOKEN !!!
# Go test should now find modules in /go/pkg/mod inherited from the builder stage
CMD ["/bin/sh", "-c", "go test -covermode=atomic -coverprofile=/app/coverdata/coverage.out ./... && echo 'Coverage data collected'"]
