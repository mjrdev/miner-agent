# Default environment of a run: the agent works on a clone of the project.
FROM node:24-slim

# Pinned so the agent's flags and event format do not change under a run.
ARG CLAUDE_CODE_VERSION=2.1.310

RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates curl jq \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
    && npm cache clean --force

COPY miner-job /usr/local/bin/miner-job

# The node user (uid 1000) owns the files written to the run directory.
USER node
WORKDIR /job
ENTRYPOINT ["miner-job"]
