# crossplane-provider-neon

A native Go [Crossplane](https://crossplane.io) provider for [Neon](https://neon.tech) serverless Postgres: manage
projects, databases and roles as Kubernetes resources, directly against the Neon REST API, with connection
credentials delivered to Kubernetes Secrets. No Terraform or Upjet in the loop.

| Kind | API group | What it manages |
|---|---|---|
| `Project` | `database.neon.crossplane.io/v1alpha1` | A Neon project (region, Postgres version) |
| `Database` | `database.neon.crossplane.io/v1alpha1` | A database on a project branch |
| `Role` | `database.neon.crossplane.io/v1alpha1` | A Postgres role on a project branch |
| `ProviderConfig` | `neon.crossplane.io/v1alpha1` | Credentials |

Package: `ghcr.io/aemendes/provider-neon-pkg`

> **Status:** `v1alpha1`.

## Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Resources](#resources)
  - [Project](#project)
  - [Role](#role)
  - [Database](#database)
- [End-to-end example](#end-to-end-example)
- [Behaviour and limitations](#behaviour-and-limitations)
- [Development](#development)
- [Release process](#release-process)
- [Security](#security) · [License](#license)

## Requirements

- A Kubernetes cluster with **Crossplane v1.17+ or v2.x**
  ([install guide](https://docs.crossplane.io/latest/software/install/)).
- A Neon **API key** ([Neon docs](https://neon.com/docs/manage/api-keys)). A personal or organization key can
  create projects; a project-scoped key can only manage databases and roles inside its project.

## Installation

The package is public on GitHub Container Registry. Pick a version from the
[packages page](https://github.com/aemendes?tab=packages&repo_name=crossplane-provider-neon) (tags are
`v0.1.<build>`); pinning by digest is recommended.

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-neon
spec:
  package: ghcr.io/aemendes/provider-neon-pkg:v0.1.X   # or @sha256:<digest>
```

```shell
kubectl apply -f provider.yaml
kubectl get providers.pkg.crossplane.io provider-neon   # INSTALLED=True, HEALTHY=True
```

Flags (set through a `DeploymentRuntimeConfig`, see the
[Crossplane docs](https://docs.crossplane.io/latest/packages/providers/#runtime-configuration)):

| Flag | Default | Description |
|---|---|---|
| `--poll` | `1m` | How often each resource is checked for drift. |
| `--sync` | `1h` | Controller manager resync period. |
| `--max-reconcile-rate` | `10` | Maximum reconciles per second. |
| `--leader-election` | `false` | Leader election, for more than one replica. |
| `--health-addr` | `:8081` | `/healthz` address. |
| `--debug` | `false` | Debug logging. |

## Configuration

Store the API key in a Secret (keep it out of git in plain text, e.g. with
[Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) or [External Secrets](https://external-secrets.io)),
and point a `ProviderConfig` at it. Resources use the `ProviderConfig` named `default` unless they set
`spec.providerConfigRef.name`.

```shell
kubectl -n crossplane-system create secret generic neon-creds --from-literal=credentials='<neon-api-key>'
```

```yaml
apiVersion: neon.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: neon-creds
      key: credentials
```

All resources are cluster-scoped Crossplane managed resources and support `providerConfigRef`, `deletionPolicy`,
`managementPolicies` and `writeConnectionSecretToRef`.

## Resources

### Project

| Field (`spec.forProvider`) | Type | Required | Description |
|---|---|---|---|
| `name` | string | no | Name in the Neon console. Defaults to `metadata.name`. Can be changed. |
| `regionId` | string | yes | e.g. `aws-eu-central-1`, `aws-us-east-2`, `azure-eastus2` ([current list](https://neon.com/docs/reference/api/regions/get-active-regions)). Immutable. |
| `pgVersion` | int | no | Postgres major version, e.g. `17`. Immutable. |

| Status (`status.atProvider`) | Description |
|---|---|
| `id` | Neon project ID (also the external name). |
| `host` | Connection host of the default branch endpoint. |
| `regionId`, `pgVersion`, `createdAt` | As reported by Neon. |

**Connection secret:** `connectionUri`, the full Postgres URI for the project's default role and database,
**including its password**. Neon returns it only when the project is created, so set `writeConnectionSecretToRef`
before the first apply.

```yaml
apiVersion: database.neon.crossplane.io/v1alpha1
kind: Project
metadata:
  name: my-app
spec:
  forProvider:
    regionId: aws-eu-central-1
    pgVersion: 17
  writeConnectionSecretToRef:
    namespace: my-app
    name: neon-project
```

### Role

A Postgres role on a branch.

| Field (`spec.forProvider`) | Type | Required | Description |
|---|---|---|---|
| `projectId` | string | one of | Neon project ID. Immutable. |
| `projectIdRef` / `projectIdSelector` | reference / selector | one of | Resolve `projectId` from a `Project` resource. |
| `branchId` | string | no | Branch; defaults to the project's default branch. Immutable. |
| `name` | string | yes | Role name. Immutable (Neon cannot rename roles). |

| Status (`status.atProvider`) | Description |
|---|---|
| `branchId` | Branch the role was created on (resolved default). |
| `protected` | Neon-managed system role that can't be deleted. |
| `createdAt` | Creation time. |

**Connection secret:** `username` and `password`. Neon returns the password only on creation, so set
`writeConnectionSecretToRef` before the first apply.

```yaml
apiVersion: database.neon.crossplane.io/v1alpha1
kind: Role
metadata:
  name: my-app-user
spec:
  forProvider:
    projectIdRef: {name: my-app}
    name: app_user
  writeConnectionSecretToRef:
    namespace: my-app
    name: neon-app-user
```

### Database

A database on a branch, owned by a role.

| Field (`spec.forProvider`) | Type | Required | Description |
|---|---|---|---|
| `projectId` | string | one of | Neon project ID. Immutable. |
| `projectIdRef` / `projectIdSelector` | reference / selector | one of | Resolve `projectId` from a `Project`. |
| `branchId` | string | no | Branch; defaults to the project's default branch. Immutable. |
| `name` | string | yes | Database name. Immutable (recreate to rename). |
| `ownerName` | string | one of | Owning role, e.g. the project's default role. |
| `ownerNameRef` / `ownerNameSelector` | reference / selector | one of | Resolve `ownerName` from a `Role`. |

| Status (`status.atProvider`) | Description |
|---|---|
| `branchId` | Branch the database was created on. |
| `ownerName` | Owner reported by Neon. |
| `createdAt` | Creation time. |

No connection secret: credentials belong to roles. Combine the `Role` secret with the project host and the
database name.

```yaml
apiVersion: database.neon.crossplane.io/v1alpha1
kind: Database
metadata:
  name: my-app-db
spec:
  forProvider:
    projectIdRef: {name: my-app}
    name: app
    ownerNameRef: {name: my-app-user}
```

## End-to-end example

A project in Frankfurt with an application role and a database it owns; the role's credentials land in the
`my-app/neon-app-user` Secret.

```yaml
apiVersion: database.neon.crossplane.io/v1alpha1
kind: Project
metadata:
  name: my-app
spec:
  forProvider:
    name: my-app
    regionId: aws-eu-central-1
    pgVersion: 17
---
apiVersion: database.neon.crossplane.io/v1alpha1
kind: Role
metadata:
  name: my-app-user
spec:
  forProvider:
    projectIdRef: {name: my-app}
    name: app_user
  writeConnectionSecretToRef: {namespace: my-app, name: neon-app-user}
---
apiVersion: database.neon.crossplane.io/v1alpha1
kind: Database
metadata:
  name: my-app-db
spec:
  forProvider:
    projectIdRef: {name: my-app}
    name: app
    ownerNameRef: {name: my-app-user}
```

Then connect with `postgresql://<username>:<password>@<status.atProvider.host of the Project>/app?sslmode=require`.

## Behaviour and limitations

- **Adopting existing resources.** A `Project` without a recorded ID is matched to an existing Neon project with
  the same name; roles and databases are addressed by name. To adopt safely, start with
  `managementPolicies: ["Observe", "LateInitialize"]`, check there is no drift, then remove it.
- **Passwords are returned once.** Neon includes passwords only in creation responses; the provider does not call
  Neon's reveal/reset endpoints. Set `writeConnectionSecretToRef` before creating, or reset the password in Neon.
- **Roles can't be updated** (Neon has no update endpoint for roles); every field is immutable.
- **References are not ownership.** Deleting a `Role` that a `Database` still uses does not block or cascade; the
  database reconcile then fails with Neon's error. Set `deletionPolicy: Orphan` on anything you can't afford to
  lose.
- **Not modelled:** branches, endpoints/compute settings, project settings beyond name/region/version.

## Development

```shell
make generate   # regenerate deepcopy code and package/crds/*.yaml
make build
make test
```

| Path | Contents |
|---|---|
| `apis/v1alpha1` | `ProviderConfig` / `ProviderConfigUsage` (`neon.crossplane.io`) |
| `apis/{project,database,role}/v1alpha1` | managed resource types and cross-resource referencers |
| `internal/clients/neon` | minimal `net/http` client for `https://console.neon.tech/api/v2` |
| `internal/controller/*` | `Observe` / `Create` / `Update` / `Delete` per kind |
| `cmd/provider` | manager entrypoint |
| `package/` | Crossplane package metadata and generated CRDs |
| `examples/` | starting manifests |

Run against a cluster from your machine: `kubectl apply -f package/crds && go run ./cmd/provider --debug`.

## Release process

[`.github/workflows/build.yaml`](.github/workflows/build.yaml) builds, vets, tests and scans every push and pull
request. On `main` it pushes the runtime image and the Crossplane package to GHCR as `v0.1.<run number>` and prints
the digest. Optionally it sends a `pin-package` repository dispatch (`client_payload: {image, ref}`) to the
repositories in the `PIN_PACKAGE_REPOS` variable, using the `PIN_DISPATCH_TOKEN` secret (a fine-grained token with
Contents: Read and write on exactly those repositories).

## Security

See [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE).
