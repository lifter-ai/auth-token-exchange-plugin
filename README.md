# Auth Token Exchange Plugin for Traefik

## Overview

The Auth Token Exchange Plugin is a custom middleware for Traefik that enhances your API gateway's authentication capabilities. It verifies incoming tokens with a configurable authentication service and, upon successful verification, exchanges them for internal tokens. These internal tokens are then passed downstream, providing essential user context to your services.

## Features

- Token verification with a configurable authentication service
- Token exchange for internal tokens
- Configurable production mode
- Built-in test token support for easy integration testing
- Retry mechanism with jitter for improved reliability

## Installation

To use this plugin with Traefik, you need to declare it in the static configuration. Add the following to your Traefik static configuration:

```yaml
experimental:
  plugins:
    authTokenExchange:
      moduleName: "github.com/mgladysheva/auth-token-exchange-plugin"
      version: "v0.1.0"
```

## Configuration

The plugin can be configured using the following options:

- `authURL`: The URL of your authentication service (required)
- `production`: Boolean flag to enable/disable production mode (default: false)

Example dynamic configuration:

```yaml
http:
  middlewares:
    authTokenExchange:
      plugin:
        auth-token-exchange-plugin:
          authURL: "https://auth.example.com/verify"
          production: true
```

## Usage

To use the plugin in your Traefik routes:

```yaml
http:
  middlewares:
    authTokenExchange:
      plugin:
        auth-token-exchange-plugin:
          authURL: "https://auth.example.com/verify"
          production: true

  routers:
    my-router:
      rule: "Host(`example.com`)"
      middlewares:
        - "authTokenExchange"
      service: "my-service"
```

## Development

To set up the development environment:

1. Clone the repository
2. Install dependencies: `go mod download`
3. Run tests: `go test -v ./...`

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.