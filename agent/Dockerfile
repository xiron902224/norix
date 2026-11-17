# Norix node agent — for remote workers or optional compose profile.
FROM rust:1.88-bookworm AS build
WORKDIR /src
COPY agent/Cargo.toml agent/Cargo.lock ./
COPY agent/src ./src
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git \
    && curl -sSL https://railpack.com/install.sh | sh \
    && rm -rf /var/lib/apt/lists/*
COPY --from=docker:27-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=build /src/target/release/norix-agent /usr/local/bin/norix-agent
WORKDIR /var/lib/norix-agent
ENTRYPOINT ["norix-agent"]
