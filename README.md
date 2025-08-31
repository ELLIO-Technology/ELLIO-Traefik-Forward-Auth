# ELLIO ForwardAuth for Traefik

Secure forward authentication middleware for Traefik that integrates with [ELLIO EDL Management Platform](https://platform.ellio.tech).

## Quick Start

### 1. Get Your Bootstrap Token

Log in to [platform.ellio.tech](https://platform.ellio.tech) and generate a bootstrap token for your deployment.

### 2. Add to Your Docker Compose

Add the ForwardAuth service to your existing `docker-compose.yml`:

```yaml
version: '3.8'

services:
  traefik:
    image: traefik:v3.0
    command:
      - "--providers.docker=true"
      - "--entrypoints.web.address=:80"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - web

  forwardauth:
    image: elliotechnology/ellio_traefik_forward_auth:latest
    environment:
      - ELLIO_BOOTSTRAP=your_bootstrap_token_here  # Replace with your token
      # Optional: Override IP header (defaults to X-Forwarded-For)
      # - IP_HEADER_OVERRIDE=X-Real-IP
    labels:
      # Define the middleware
      - "traefik.http.middlewares.ellio-auth.forwardAuth.address=http://forwardauth:8080/auth"
    networks:
      - web

  # Your protected service
  your-app:
    image: your-app:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.your-app.rule=Host(`app.example.com`)"
      - "traefik.http.routers.your-app.middlewares=ellio-auth"  # Apply the middleware
    networks:
      - web

networks:
  web:
    driver: bridge
```

That's it! Your services are now protected by ELLIO EDL.

## Understanding the Labels

The ForwardAuth middleware is configured through Traefik labels:

### On the ForwardAuth service

- **`traefik.http.middlewares.ellio-auth.forwardAuth.address`**  
  Tells Traefik where to send authentication requests. The middleware validates every request through this endpoint before allowing access to protected services.

### On your protected services

- **`traefik.http.routers.your-app.middlewares=ellio-auth`**  
  Applies the authentication middleware to your service. Every incoming request will be validated against the EDL before reaching your application.

## How EDL Metadata Controls Access

The ForwardAuth middleware adapts its behavior based on your EDL deployment configuration in the ELLIO platform:

### EDL Purpose Types

- **Allowlist**: Only IP addresses in the EDL are granted access. All other IPs are blocked.
- **Blocklist**: IP addresses in the EDL are denied access. All other IPs are allowed.
- **Other/Custom**: Defaults to blocklist behavior for security.

### Supported IP Formats

The EDL supports multiple IP address formats:

- IPv4 addresses (e.g., `192.168.1.1`)
- IPv6 addresses (e.g., `2001:db8::1`)
- CIDR notation for both IPv4 and IPv6 (e.g., `10.0.0.0/8`, `2001:db8::/32`)

### Automatic Configuration

- **Update Frequency**: Automatically synchronized from your EDL metadata settings
- **Dynamic Updates**: The middleware fetches EDL updates at the configured interval without service interruption
- **Zero-downtime**: Updates are applied atomically with no impact on active connections

### Failsafe Behavior

In the event of deployment issues:

- **Disabled Deployment**: If the deployment is disabled in the ELLIO platform, the middleware falls back to allowing all traffic to prevent service disruption
- **Deleted Deployment**: Similar failsafe applies - all traffic is allowed to maintain availability
- **Network Issues**: The last successfully fetched EDL remains active until connectivity is restored

## How It Works

1. **Request arrives** at Traefik for your protected service
2. **Traefik forwards** the request to ForwardAuth middleware
3. **ForwardAuth extracts** the client IP from `X-Forwarded-For` header (or custom header if configured)
4. **IP validation** against the current EDL based on the configured purpose (allowlist/blocklist)
5. **Access decision**: Returns 200 (allowed) or 403 (denied) to Traefik
6. **EDL synchronization** occurs automatically based on metadata configuration

## Support

- **Issues**: [GitHub Issues](https://github.com/ELLIO-Technology/ellio_traefik_forward_auth/issues)
- **Platform**: [platform.ellio.tech](https://platform.ellio.tech)

## License

Apache License 2.0 - see [LICENSE](LICENSE) file for details.

## Acknowledgments

This project is built with the following open-source technologies:

- **[Traefik](https://traefik.io/)** - Cloud-native reverse proxy and load balancer
- **[golang-jwt/jwt](https://github.com/golang-jwt/jwt)** - JWT implementation for Go
- **[Prometheus Client](https://github.com/prometheus/client_golang)** - Prometheus instrumentation library
- **[go4.org/netipx](https://github.com/go4org/netipx)** - IP address and CIDR manipulation library
- **[Go](https://golang.org/)** - The Go programming language

### Trademarks and Copyrights

- ELLIO® is a registered trademark of ELLIO Technology s.r.o.
- Traefik® is a registered trademark of Traefik Labs.
- The Go gopher was designed by Renée French.
- All other trademarks, service marks, and trade names referenced herein are the property of their respective owners.

---

Copyright © ELLIO Technology s.r.o. | Part of the [ELLIO EDL Management Platform](https://platform.ellio.tech)
