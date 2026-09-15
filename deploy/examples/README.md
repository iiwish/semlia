# Deployment Examples

These files illustrate deployment boundaries, not a configured environment. Keep resolved configuration, credentials and deployment records outside the repository. `deployment.json` is a documentation example with placeholders, not an executable platform manifest.

## Container

`Dockerfile` packages a previously built Linux executable, migrations and application data directories from `build/container/`. Build it from the repository root. Do not put secrets or source customer data in that directory or image.

## Compose

`compose.yaml` requires explicit image identity, project name, build version, allowed origins, loopback port, runtime and migration environment files, host directories and an existing database network. `pull_policy: never` requires the approved immutable image to be loaded or pulled separately. Render the configuration with `docker compose -f deploy/examples/compose.yaml config` and review it before starting services.

The runtime and migration environment files must use separate least-privilege database credentials and an explicit connection budget. This example does not create a database, role, shared network or secret. The host directories must exist and have appropriate ownership. Server and worker share artifacts; only the worker has write access to the content directory. Adjust resource budgets to the approved environment without removing the read-only root filesystem or loopback-only listener.

## Reverse Proxy

`nginx.conf.template` assumes a trusted TLS-terminating proxy connects to the origin over loopback and supplies the forwarded protocol header. It is not a standalone public TLS configuration or an identity access gateway. Choose strong application credentials and an appropriate external access policy before exposing an installation.

Render only the two declared template variables, preserving nginx's request variables:

```sh
envsubst '${SEMLIA_HOSTNAME} ${SEMLIA_PORT}' < deploy/examples/nginx.conf.template
```

Use a validated hostname and numeric loopback port, inspect the rendered result, and run the operator's configuration validation before installation. Match the application's allowed HTTPS origin to the chosen hostname. The public proxy does not expose health or metrics endpoints.
