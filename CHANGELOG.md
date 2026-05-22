# Changelog — probe-core

All notable changes to **fluid-pub/probe-core** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Downstream probes (submodule or tagged module) should note **`probe-core`** upgrades in their own changelogs.

## [Unreleased]

### Added

- **`controlplane.HTTPClient`**: HTTP transport for **`/probes/register`**, **`/probes/ping`**, **`/probes/v1/ingest`**, **`GET /probes/config`**, **`POST /probes/v1/schema`**.
- **`ResolvedBaseURL`** / **`BaseURLFromWebSocketURL`** to migrate legacy `websocket_url` settings without operator secret changes.

### Changed

- **`NewClientFromConfig`** uses HTTP by default (WebSocket client remains in the tree but is no longer selected by the factory).

## [0.1.0] - 2026-05-21

### Added

- Initial published shared library: probe lifecycle, state manager, WebSocket control plane client, schema load/push, and runtime config merge helpers.
