# Auth Token Exchange Plugin for Traefik

## Overview

The Auth Token Exchange Plugin is a custom middleware for Traefik that enhances your API gateway's authentication capabilities. It verifies incoming tokens with a configurable authentication service and, upon successful verification, exchanges them for internal tokens. These internal tokens are then passed downstream, providing essential user context to your services.

## Features

- Token verification with a configurable authentication service
- Token exchange for internal tokens
- Configurable production mode
- Built-in test token support for easy integration testing
- Retry mechanism with jitter for improved reliability
- Adds `X-User-Id`, `X-User-Info`, and `X-Request-Id` headers to authenticated requests, and strips the incoming `Authorization` header

## Installation

To use this plugin with Traefik, you need to declare it in the static configuration. 

Add the following to your Traefik static configuration:

```yaml
experimental:
  plugins:
    authTokenExchange:
      moduleName: "github.com/lifter-ai/auth-token-exchange-plugin"
      version: "v0.1.5"
```

## Configuration

The plugin can be configured using the following options:

- `authURL`: The URL of your authentication service (required)
- `production`: Boolean flag to enable/disable production mode (default: false)

## Usage with Traefik Helm Chart

To use this plugin with a Traefik Helm chart, you can include it in your `values.yaml` file or directly in your Helm command. Here's an example configuration:

```yaml
providers:
  kubernetesCRD:
    enabled: true
    allowCrossNamespace: false
    allowExternalNameServices: false
ingressRoute:
  dashboard:
    enabled: true
experimental:
  plugins:
    auth-token-exchange-plugin:
      moduleName: github.com/lifter-ai/auth-token-exchange-plugin
      version: v0.1.5
extraObjects:
  - apiVersion: traefik.io/v1alpha1
    kind: Middleware
    metadata:
      name: auth-token-exchange
      namespace: your_namespace
    spec:
      plugin:
        auth-token-exchange-plugin:
          authURL: "your endpoint for GET request to verify the token"
          production: false
  - apiVersion: traefik.io/v1alpha1
    kind: IngressRoute
    metadata:
      name: your_service
    spec:
      entryPoints:
        - web
      routes:
        - match: PathPrefix(`/`)
          kind: Rule
          services:
            - name: your_service
              port: your_service_port
          middlewares:
            - name: auth-token-exchange
              namespace: your_namespace
```

In this configuration:

1. The plugin is declared in the `experimental.plugins` section.
2. A Middleware resource is created to configure the plugin.
3. An IngressRoute is defined to use the middleware for a specific service.

Remember to replace `your_namespace`, `your_service`, and `your_service_port` with your actual values.

## Headers

On a successful (200) verification the plugin adds these headers to the forwarded request:

- `X-User-Id`: the `id` field extracted from the authentication-service response body
- `X-User-Info`: base64-encoded JSON of the **entire** authentication-service response body
- `X-Request-Id`: a unique UUID (v7) generated per request

It also **deletes** the incoming `Authorization` header, so downstream services never see the original credential.

These headers let downstream services identify the user and trace requests. For how the plugin is wired into a real deployment — and exactly which services receive `X-User-Info` — see [doc/consumers.md](doc/consumers.md).

## Development

To set up the development environment:

1. Clone the repository
2. Install dependencies: `go mod download`
3. Run tests: `go test -v ./...`

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.