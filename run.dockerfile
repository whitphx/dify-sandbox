ARG BASE_IMAGE=python:3.10-slim-bookworm
ARG GOLANG_VERSION=1.24.11

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
# Install libseccomp 2.6.0 from Debian trixie to match the version used in the builder
# (golang:1.24.x is based on Debian trixie which has libseccomp 2.6.0)
RUN echo 'deb http://deb.debian.org/debian trixie main' >> /etc/apt/sources.list \
    && apt-get update && apt-get install -y -t trixie \
    libseccomp2 \
    && apt-get install -y \
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

# Create python path expected by config
RUN mkdir -p /opt/python/bin && ln -s $(which python3) /opt/python/bin/python3

# Expose port
EXPOSE 8194

# Run server
CMD ["./main"]
