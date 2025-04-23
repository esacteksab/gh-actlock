FROM golang:1.24.2-bookworm@sha256:00eccd446e023d3cd9566c25a6e6a02b90db3e1e0bbe26a48fc29cd96e800901 AS builder

# Set GOMODCACHE explicitly (still good practice)
ENV GOMODCACHE=/go/pkg/mod

# Keep this layer cached if possible
RUN apt update && apt install -y unzip wget git \
  && wget https://github.com/cli/cli/releases/download/v2.69.0/gh_2.69.0_linux_amd64.deb \
  && dpkg -i gh_2.69.0_linux_amd64.deb && rm gh_2.69.0_linux_amd64.deb \
  && wget https://releases.hashicorp.com/terraform/1.5.7/terraform_1.5.7_linux_amd64.zip \
  && unzip terraform_1.5.7_linux_amd64.zip && rm terraform_1.5.7_linux_amd64.zip \
  && mv terraform /usr/bin/terraform && chmod +x /usr/bin/terraform \
  && wget https://github.com/opentofu/opentofu/releases/download/v1.9.0/tofu_1.9.0_amd64.deb \
  && dpkg -i tofu_1.9.0_amd64.deb && rm tofu_1.9.0_amd64.deb

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

# Go test should now find modules in /go/pkg/mod inherited from the builder stage
CMD ["/bin/sh", "-c", "go test -covermode=atomic -coverprofile=/app/coverdata/coverage.out ./... && echo 'Coverage data collected'"]
