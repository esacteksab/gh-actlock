FROM esacteksab/go:1.25.3-2025-10-31@sha256:540ed5c9e34a12e4cff46d914322f39100bb49eb7df4404b85811d2dec19d021 AS builder

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
