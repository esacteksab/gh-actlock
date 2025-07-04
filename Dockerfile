FROM esacteksab/go:1.24.4-2025-06-27@sha256:9c6f841f1afd8c2e9f4b729ca2f19a8a51680bf97f5f80cf6886c7de2e0056d8 AS builder

# Set GOMODCACHE explicitly (still good practice)
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

# No need to set GOMODCACHE again, inherited from builder
# No need to set WORKDIR again, inherited from builder

RUN mkdir -p /app/coverdata
ENV GOCOVERDIR=/app/coverdata

# !!! DO NOT ADD A `-v` TO THIS CMD AND ALLOW TO RUN IN GITHUB ACTIONS. YOU **WILL** LEAK GITHUB_TOKEN !!!
# Go test should now find modules in /go/pkg/mod inherited from the builder stage
CMD ["/bin/sh", "-c", "go test -covermode=atomic -coverprofile=/app/coverdata/coverage.out ./... && echo 'Coverage data collected'"]
