ARG BASE=ghcr.io/actions/actions-runner:latest

FROM ${BASE}

COPY preload.js /opt/cache-redirect.js

ENV NODE_OPTIONS=--require=/opt/cache-redirect.js \
    ACTIONS_CACHE_SERVICE_V2=true
