# Changelog — probe-core

All notable changes to **fluid-pub/probe-core** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Downstream probes (submodule or tagged module) should note **`probe-core`** upgrades in their own changelogs.

## [Unreleased]

### Added

- **`enroll`**: HTTP client for **`POST /api/v1/enrollment/enroll`** (mirror of `fluid/agents/core/enroll`; keep both packages aligned when changing enrollment behavior).
- **`controlplane.HTTPClient`**: HTTP transport for **`/probes/register`**, **`/probes/ping`**, **`/probes/v1/ingest`**, **`GET /probes/config`**, **`POST /probes/v1/schema`**.
- **`controlplane.RuntimeSync`**: fetch runtime config at startup and reload on ping **`configuration_changed`**.
- **`RuntimeConfig`**: entity list overlay only (`data.entities`); integration-specific fields (e.g. Debian `collection` / `files`) are parsed in each probe repository.

### Changed

- **`NewClientFromConfig`** requires **`controlplane.base_url`** (http/https only).

### Removed

- WebSocket control plane client and **`websocket_url`** configuration; **`github.com/gorilla/websocket`** dependency.

## [0.1.0] - 2026-05-21

### Added

- Initial published shared library: probe lifecycle, state manager, control plane client, schema load/push, and runtime config merge helpers.
