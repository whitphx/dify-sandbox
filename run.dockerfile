ARG BASE_IMAGE=python:3.10-slim-bookworm
ARG GOLANG_VERSION=1.24.9

# Builder stage
FROM golang:${GOLANG_VERSION} AS builder
ARG TARGETARCH
WORKDIR /app
COPY . /app
RUN apt-get update && apt-get install -y pkg-config gcc libseccomp-dev
RUN touch internal/core/runner/python/python.so \
    && touch internal/core/runner/nodejs/nodejs.so \
    && go mod tidy
RUN bash ./build/build_${TARGETARCH}.sh

# Runtime stage
FROM ${BASE_IMAGE} AS runner
RUN apt-get update && apt-get install -y \
    libseccomp-dev \
    curl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
# Copy binary
COPY --from=builder /app/main /app/main
# Copy config
COPY conf/config.yaml /app/conf/config.yaml
# Copy dependencies list
COPY dependencies/python-requirements.txt /app/dependencies/python-requirements.txt

# Install Python dependencies
RUN pip3 install --no-cache-dir httpx==0.27.2 requests==2.32.3 jinja2==3.1.6 PySocks httpx[socks]

# Ensure storage directory exists
RUN mkdir -p /tmp/dify_sandbox_data

# Symlink python3 to /usr/bin/python3
RUN ln -s $(which python3) /usr/bin/python3

# Expose port
EXPOSE 8194

# Run server
CMD ["./main"]
