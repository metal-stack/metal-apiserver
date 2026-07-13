# RBAC, Authentication & Token Management

## Table of Contents

- [1. Login](#1-login)
- [2. Token Types](#2-token-types)
- [3. Creating User Tokens (Login Flow)](#3-creating-user-tokens-login-flow)
- [4. Creating API Tokens](#4-creating-api-tokens)
- [5. Roles Overview](#5-roles-overview)
- [6. Permissions Table](#6-permissions-table)
- [7. Subject Scoping](#7-subject-scoping)
- [8. Token Lifecycle](#8-token-lifecycle)

---

## 1. Login

Authentication is performed via **OpenID Connect (OIDC)**. The metal-apiserver supports OIDC providers configured at deployment time (e.g. GitHub, GitLab, or any generic OIDC provider). The login flow uses the [goth](https://github.com/markbates/goth) library for provider integration.

### 1.1 How Login Works

1. The user navigates to the login endpoint, which initiates the OIDC authorization flow.
2. The user is redirected to the OIDC provider's login page.
3. After the provider authenticates the user, the user is redirected back via the provider callback URL.
4. The server validates the OIDC tokens, extracts the user's unique login identifier (the `sub` claim, configurable via `uniqueUserKey`), and looks up or creates the corresponding tenant.
5. A **user token** is generated and appended to the redirect URL as a `token` query parameter (e.g. `?token=eyJhbGc...`).
6. With such a **user token** a user can create **api tokens** for specific use cases with restricted capabilities which services can be called.

### 1.2 Login HTTP Endpoints

| Endpoint         | Method | Description                                                                                                                                                                       |
|------------------|--------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `/auth/login`    | GET    | Starts the OIDC authorization flow. An optional `redirect-url` query parameter specifies where to return after login.                                                             |
| `/auth/callback` | GET    | OIDC provider callback. The server receives the authorization code, exchanges it for tokens, creates the user token, and redirects to the `redirect-url` with the token appended. |
| `/auth/logout`   | GET    | Terminates the session. Optionally redirects to the provider's end-session URL.                                                                                                   |

### 1.3 Login Constraints

- **Tenant auto-creation**: On first login, if the user does not have a tenant, one is automatically created with the same ID as the user's login. The user is granted `OWNER` role on their own tenant.
- **Provider tenant**: Users who are members of the configured provider tenant receive admin-level roles (`ADMIN_ROLE_EDITOR` or `ADMIN_ROLE_VIEWER`), depending on their provider tenant membership level. These admin roles must be explicitly ordered and are not granted automatically on login.
- **Redirect URL validation**: The `redirect-url` parameter must match a configured allowlist of allowed redirect URLs (same scheme and hostname).

---

## 2. Token Types

| Token Type     | Constant          | Purpose                                                                                                                                                                                                                                                                                                                                |
|----------------|-------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **User Token** | `TOKEN_TYPE_USER` | Created during the interactive login flow. Contains the user's implicit roles from the tenant-API-server. Used by human users interacting with the console/UI. User tokens do **not** store `AdminRole`, `TenantRoles`, `ProjectRoles`, or `Permissions` -- these are resolved dynamically from the tenant-API-server at request time. |
| **API Token**  | `TOKEN_TYPE_API`  | Created programmatically via the Token API (`TokenService.Create`, `TokenService.Refresh`, or the CLI). Used by machines, CI/CD pipelines, and automation. Stores all permissions explicitly embedded in the JWT.                                                                                                                      |

**Key difference**: A User Token carries dynamic role enforcement resolved at runtime from the tenant-API-server (the `tenant-apiserver` that manages tenant/project memberships). An API Token carries static, self-contained permissions directly in the JWT payload. This means API tokens can be given fine-grained custom permissions that are not tied to tenant-project membership.

---

## 3. Creating User Tokens (Login Flow)

### 3.1 Via the Web UI (Console Login)

1. Open the web UI and navigate to the login page.
2. Authenticate with your OIDC provider.
3. Upon successful authentication, you are redirected with a token appended to the URL.
4. The token is automatically used for subsequent authenticated API calls or UI operations.

### 3.2 Via the CLI (`metalctl login`)

1. Run `metalctl login --api-url <reachable apiserver endpoint> --provider openid-connect`
2. A browser will open and you are prompted for your credentials.
3. With successful authentication, you are redirected with the token and the cli will store them in ~/.metal-stack/config.yaml
4. Subsequent calls with metalctl will be authenticated with the token stored in the configuration.
5. api-url and provider are stored in the config.yaml. Further logins do not require them anymore

### 3.3 Via the CLI (`api-server token create`)

The server provides a CLI command for creating tokens programmatically without web-based OIDC:

```bash
api-server token create <subject>
```

Internally this calls `CreateApiTokenWithoutPermissionCheck` which:

- Uses the default expiration if none is specified.
- Creates a `TOKEN_TYPE_API` token.
- Can optionally accept `--project-roles`, `--tenant-roles`, `--admin-role`, `--infra-role`, `--machine-roles`, `--permissions`, and `--expires` flags.

A cronjob with the apiserver images is executed every 8h and create a `ADMIN_ROLE_EDITOR` and a `ADMIN_ROLE_VIEWER` token and store it in a Secret `metal-apiserver-admin-token`
The tokens in this secret expire after 16h and can be used e.g. for IaC automation jobs which need a token with admin role to create other tokens for services for example.

---

## 4. Creating API Tokens

API tokens are managed through the Token Service API. They are JWTs that carry explicit role assignments and permissions.

### 4.1 Token Service Endpoints

| Endpoint (connect-procedure) | Description                                                                                                                                     |
|------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| `TokenService.Create`        | Creates a new API token with the specified roles and permissions.                                                                               |
| `TokenService.Refresh`       | Refreshes an existing token (re-issue with the same permissions and duration, but a new JWT and secret).                                        |
| `TokenService.Update`        | Updates an existing API token's roles, permissions, or description. Only updates to roles/permissions the caller already possesses are allowed. |
| `TokenService.Get`           | Retrieves a specific token by UUID (only its own tokens).                                                                                       |
| `TokenService.List`          | Lists all tokens belonging to the authenticated user.                                                                                           |
| `TokenService.Revoke`        | Revokes (deletes) a token.                                                                                                                      |

### 4.2 Token Creation Request Fields

When calling `TokenService.Create`, you must provide a `TokenServiceCreateRequest` with the following optional fields:

| Field           | Type                       | Description                                                                                                                                                              |
|-----------------|----------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `expires`       | `google.protobuf.Duration` | Token validity period. Must not exceed `MaxTokenExpiration` (server configuration).                                                                                      |
| `description`   | `string`                   | Human-readable description for identification.                                                                                                                           |
| `admin_role`    | `AdminRole`                | Admin-level role (see Section 5).                                                                                                                                        |
| `infra_role`    | `InfraRole`                | Infrastructure-level role (see Section 5).                                                                                                                               |
| `project_roles` | `map<string, ProjectRole>` | Map of project ID to role (e.g. `{"my-project": "PROJECT_ROLE_OWNER"}`).                                                                                                 |
| `tenant_roles`  | `map<string, TenantRole>`  | Map of tenant ID to role (e.g. `{"my-tenant": "TENANT_ROLE_EDITOR"}`). Use `"*"` for any/unknown tenants.                                                                |
| `machine_roles` | `map<string, MachineRole>` | Map of machine UUID to role (e.g. `{"<uuid>": "MACHINE_ROLE_OWNER"}`). Use `"*"` for any machine.                                                                        |
| `permissions`   | `MethodPermission[]`       | Custom fine-grained permissions, each specifying a `subject`, `methods` list (e.g. `[{"subject": "my-project", "methods": ["metalstack.api.v2.MachineService/List"]}]`). |

### 4.3 Permission Elevation Prevention

Token creation enforces a **no-elevation** rule: the caller's token must already possess sufficient permissions to authorize every requested role/permission in the new token. This prevents privilege escalation. The validation works as follows:

1. If the requesting user is a member of the **provider tenant**, they can request a admin only up to the level of their own provider-tenant membership, see table 5.1 below.
2. For project-scoped roles, the user must already have a role in the target project (or use `"*"` for future/unknown projects). The same applies to tenants and machines.
3. Custom `MethodPermission` entries must reference methods the user can already call.
4. Every requested subject (tenant, project, machine) must either be one the user already has access to, or `"*"` (wildcard).

---

## 5. Roles Overview

Roles in the metal-apiserver follow a hierarchical structure across multiple scopes:

### 5.1 Admin Roles (Unscoped -- Apply Globally)

Admin roles are the highest privilege level and apply to **all** subjects.

| Role                | Description                                                                                                                                                                                                                               |
|---------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ADMIN_ROLE_EDITOR` | Full editor access to all resources, all projects, all tenants, all components, all partitions, all switches, all sizes, all images, all filesystem layouts, all IPAM, all tasks, all audit traces. Can read, create, update, and delete. |
| `ADMIN_ROLE_VIEWER` | Read-only access across all resources. Cannot create, update, or delete anything.                                                                                                                                                         |

**Important**: Only members of the **provider tenant** can hold admin roles. The provider tenant role determines the maximum admin role:

- `TENANT_ROLE_OWNER` in the provider tenant --> maximum admin role is `ADMIN_ROLE_EDITOR`
- `TENANT_ROLE_EDITOR` or `TENANT_ROLE_VIEWER` in the provider tenant --> maximum admin role is `ADMIN_ROLE_VIEWER`

### 5.2 Infrastructure Roles (Unscoped -- Infrastructure Level)

Infra roles apply globally to infrastructure-managed resources.

### 5.3 Tenant Roles (Tenant-Scoped)

| Role                 | Description                                        |
|----------------------|----------------------------------------------------|
| `TENANT_ROLE_OWNER`  | Full control over the tenant and all its projects. |
| `TENANT_ROLE_EDITOR` | Can manage resources within the tenant.            |
| `TENANT_ROLE_VIEWER` | Read-only access within the tenant.                |

### 5.4 Project Roles (Project-Scoped)

| Role                  | Description                              |
|-----------------------|------------------------------------------|
| `PROJECT_ROLE_OWNER`  | Full control over the project.           |
| `PROJECT_ROLE_EDITOR` | Can manage resources within the project. |
| `PROJECT_ROLE_VIEWER` | Read-only access within the project.     |

### 5.5 Machine Roles (Machine-Scoped, per UUID)

| Role                  | Description                             |
|-----------------------|-----------------------------------------|
| `MACHINE_ROLE_OWNER`  | Full control over the specific machine. |
| `MACHINE_ROLE_EDITOR` | Can manage the machine.                 |
| `MACHINE_ROLE_VIEWER` | Read-only access to the machine.        |

---

## 6. Permissions Table

The following table maps each role to the API methods (gRPC connect procedures) it can access. A method marked with `[*]` is accessible for **all subjects**, while others are scoped to the corresponding resource type.

### 6.1 `ADMIN_ROLE_EDITOR`

Grants full access to **all methods on all subjects**:

- Every API method (public, self-scoped, tenant-scoped, project-scoped, and infra-scoped)
- Read (+ create, update, delete) on all resources

### 6.2 `ADMIN_ROLE_VIEWER`

Grants **read-only** access to all methods on all subjects, including:

- `APIService/GetEndpoints` (public)
- `TokenService/List` (self)
- `TokenService/Get` (self)
- `AuthService/GetLoginState` (self)
- `UserService/Get` (self)
- `UserService/List` (self)
- All audit trace GET/LIST methods
- All APIService methods
- All Machine Service (admin) methods
- All Component Service methods
- All Partition Service methods
- All Switch Service methods
- All Size Service methods
- All Image Service methods
- All FilesystemLayout Service methods
- All IPAM Service methods
- All Task Service methods
- All IP Service (admin) methods
- All SizeReservation Service methods
- All SizeImageConstraint Service methods
- All Network Service (admin) methods
- Plus all project-scoped and tenant-scoped methods

### 6.3 Infra Roles (Infra-Scoped)

These grant access to infrastructure-level methods on **all subjects**.

| Method                                              | `INFRA_ROLE_EDITOR` | `INFRA_ROLE_VIEWER` |
|-----------------------------------------------------|:-------------------:|:-------------------:|
| `/metalstack.infra.v2.BMCService/UpdateBMCInfo`     |         +R          |                     |
| `/metalstack.infra.v2.BMCService/WaitForBMCCommand` |         +R          |                     |
| `/metalstack.infra.v2.BMCService/BMCCommandDone`    |         +R          |                     |
| `/metalstack.infra.v2.BootService/Dhcp`             |         +R          |                     |
| `/metalstack.infra.v2.BootService/Boot`             |         +R          |                     |
| `/metalstack.infra.v2.ComponentService/Ping`        |         +R          |                     |
| `/metalstack.infra.v2.EventService/Send`            |         +R          |                     |
| `/metalstack.infra.v2.SwitchService/Get`            |         +R          |                     |
| `/metalstack.infra.v2.SwitchService/Register`       |         +R          |                     |
| `/metalstack.infra.v2.SwitchService/Heartbeat`      |         +R          |                     |
| `/metalstack.infra.v2.SwitchService/Heartbeat`      |         +R          |                     |
| `/metalstack.infra.v2.SwitchService/Get`            |                     |          R          |

### 6.4 Tenant Roles (Tenant-Scoped)

// FIXME review all tables below

These grant access to tenant-scoped methods, scoped to the **subject** being the tenant ID.

| Method                             | `TENANT_ROLE_OWNER` | `TENANT_ROLE_EDITOR` | `TENANT_ROLE_VIEWER` |
|------------------------------------|:-------------------:|:--------------------:|:--------------------:|
| `TenantService/Get`                |         +R          |          +R          |          +R          |
| `TenantService/List`               |         +R          |          +R          |          +R          |
| `TenantService/Create`             |         +R          |          -           |          -           |
| `TenantService/Update`             |         +R          |          -           |          -           |
| `TenantService/Delete`             |         +R          |          -           |          -           |
| `TenantService/AddMember`          |         +R          |          -           |          -           |
| `TenantService/RemoveMember`       |         +R          |          -           |          -           |
| `TenantService/CreateProject`      |         +R          |          -           |          -           |
| `TenantService/UpdateLabels`       |         +R          |          -           |          -           |
| `ProjectService/Get`               |         +R          |          +R          |          +R          |
| `ProjectService/List`              |         +R          |          +R          |          +R          |
| `ProjectService/Create`            |         +R          |          +R          |          -           |
| `ProjectService/Update`            |         +R          |          +R          |          -           |
| `ProjectService/Delete`            |         +R          |          +R          |          -           |
| `ProjectService/GetProjectQuota`   |         +R          |          +R          |          +R          |
| `ProjectService/AddMember`         |         +R          |          +R          |          -           |
| `ProjectService/RemoveMember`      |         +R          |          +R          |          -           |
| `ProjectService/UpdateLabels`      |         +R          |          +R          |          -           |
| Machine/Network/IP/Image/Size/etc. |     All methods     |  See project roles   |      Read-only       |

### 6.5 Project Roles (Project-Scoped)

These grant access to project-scoped methods, scoped to the **subject** being the project ID.

| Method                       | `PROJECT_ROLE_OWNER` | `PROJECT_ROLE_EDITOR` | `PROJECT_ROLE_VIEWER` |
|------------------------------|:--------------------:|:---------------------:|:---------------------:|
| `MachineService/Get`         |          +R          |          +R           |          +R           |
| `MachineService/Create`      |          +R          |          +R           |           -           |
| `MachineService/Update`      |          +R          |          +R           |           -           |
| `MachineService/List`        |          +R          |          +R           |          +R           |
| `MachineService/Delete`      |          +R          |          +R           |           -           |
| `MachineService/Allocate`    |          +R          |          +R           |           -           |
| `MachineService/SetState`    |          +R          |          +R           |           -           |
| `MachineService/BMCCommand`  |          +R          |           -           |           -           |
| Machine Console Password     |          +R          |           -           |           -           |
| Network Service CRD          |          +R          |          +R           |          +R           |
| Network Create/Update/Delete |          +R          |          +R           |           -           |
| IP Service CRD               |          +R          |          +R           |          +R           |
| IP Allocate/Release          |          +R          |          +R           |           -           |
| SizeReservation Service      |          +R          |          +R           |          +R           |
| VPN Service                  |          +R          |          +R           |          +R           |

### 6.6 Machine Roles (Machine-Scoped)

These grant access to machine-service methods scoped to a specific machine UUID.

| Method                                | `MACHINE_ROLE_OWNER` | `MACHINE_ROLE_EDITOR` | `MACHINE_ROLE_VIEWER` |
|---------------------------------------|:--------------------:|:---------------------:|:---------------------:|
| `MachineService/Get` (target machine) |          +R          |          +R           |          +R           |
| `MachineService/List`                 |          +R          |          +R           |          +R           |
| `MachineService/SetState`             |          +R          |          +R           |           -           |
| `MachineService/ConsolePassword`      |          +R          |          +R           |           -           |
| `MachineService/GetConsolePassword`   |          +R          |          +R           |           -           |
| `MachineService/BMCCommand`           |          +R          |           -           |           -           |
| `MachineService/GetBMC`               |          +R          |           -           |           -           |
| `MachineService/ListBMC`              |          +R          |           -           |           -           |
| `MachineService/Issues`               |          +R          |          +R           |          +R           |

### 6.7 Public & Self Methods

| Method                      | Access                   | Description                                 |
|-----------------------------|--------------------------|---------------------------------------------|
| `AuthService/GetLoginState` | Public (unauthenticated) | Returns login state and available providers |
| `TokenService/Create`       | Public (unauthenticated) | Create a new user token via OIDC callback   |
| `APIService/GetEndpoints`   | Public (unauthenticated) | Lists all available API endpoints           |
| `UserService/Get`           | Self-scoped              | Get current user's profile                  |
| `UserService/List`          | Self-scoped              | List users                                  |
| `TokenService/Get`          | Self-scoped              | Get a token by UUID                         |
| `TokenService/List`         | Self-scoped              | List user's own tokens                      |

---

## 7. Subject Scoping

Every method in the metal-apiserver is categorized by its **scope**, which determines how the subject (resource identifier) is checked:

| Category    | Method Scoping                                  | Authorization Logic                                                                 |
|-------------|-------------------------------------------------|-------------------------------------------------------------------------------------|
| **Public**  | No subject required                             | Accessible to everyone, including unauthenticated requests                          |
| **Self**    | Subject = authenticated user token's user field | Only the token's owner can call. Used for `/user`, `/token` operations              |
| **Project** | Subject = project ID from request               | Token must have project-level role for that specific project ID (or wildcard `*`)   |
| **Tenant**  | Subject = tenant ID from request                | Token must have tenant-level role for that specific tenant ID (or wildcard `*`)     |
| **Machine** | Subject = machine UUID from request             | Token must have machine-level role for that specific machine UUID (or wildcard `*`) |

### Wildcard `*` Subject

When a token contains `*` as a subject for a given role, it means the token has access to **all subjects** of that scope. For example:

- `project_roles: {"*": "PROJECT_ROLE_EDITOR"}` grants project editor access to all current and future projects.
- `tenant_roles: {"*": "TENANT_ROLE_EDITOR"}` grants tenant editor access to all current and future tenants.

### Permission Elevation and Wildcards

During token creation, a user cannot specify a wildcard `*` unless their own token also has `*` for the same scope, OR the user has the specific role on that subject. Wildcards allow tokens to work with resources that don't exist yet (e.g. future projects).

---

## 8. Token Lifecycle

### 8.1 Token Structure

Tokens are **JWT (JSON Web Tokens)** signed with an X.509 private key. Each token carries:

- `token_uuid` (`uuid`): Unique identifier, used for revocation
- `user` (`string`): The user/login who owns this token
- `iss` (`string`): Token issuer
- `exp` (`timestamp`): Expiration time
- `iat` (`timestamp`): Issued-at time
- `token_type` (`TokenType_enum`): Whether `TOKEN_TYPE_USER` or `TOKEN_TYPE_API`
- `admin_role` (`AdminRole_enum`): Admin-level role
- `infra_role` (`InfraRole_enum`): Infrastructure-level role
- `project_roles` (`map<string, ProjectRole_enum>`): Project-scoped roles
- `tenant_roles` (`map<string, TenantRole_enum>`): Tenant-scoped roles
- `machine_roles` (`map<string, MachineRole_enum>`): Machine-scoped roles
- `permissions` (`MethodPermission[]`): Custom fine-grained permissions
- `description` (`string`): Human-readable label
- `labels` (`map<string, string>`): Token metadata labels

### 8.2 Token Expiration

- **User tokens** created via login do not have an explicit expiration (use default).
- **API tokens** have a configurable expiration set during creation. The maximum allowed expiration is defined by the server's `MaxTokenExpiration` setting.

### 8.3 Token Refresh

The **Refresh** operation creates a brand new JWT with the exact same roles and permissions as the original token, giving the consumer a fresh `exp` time while retaining all capabilities. It requires the calling token to already have the permissions to authorize the requested roles. Token must contain the required permissions `TokenService/Refresh` to call the refresh endpoint.

### 8.4 Token Revocation

To revoke a token, call `TokenService.Revoke` with the token's UUID. After revocation, the token is removed from the token store and all subsequent API calls using it will fail with an authorization error. The revocation is effective immediately on the next request.

### 8.5 Certificate Rotation

Tokens are signed using X.509 certificates managed by the server's certificate store. During rotation, new tokens are signed with the latest private key while existing tokens remain valid until they expire (since old certificates are still used for verification). The signing certificate is fetched at token creation time from `t.certs.LatestPrivate(ctx)` (see `pkg/service/api/token/token-service.go:85`).

### 8.6 Token Storage

Each user can manage their own tokens. Tokens are listed with `TokenService/List`. Users can only see, update, and delete their own tokens. Admin users can create/list/delete tokens on behalf of other users.

---

## Appendix A: Token Creation Decision Flow

When a user creates an API token, this decision flow determines whether the operation is allowed:

```plain
1. Is the calling token a valid USER or API token type?
   No  --> error: "invalid token type for token creation"
   Yes --> continue

2. Does the expiration exceed MaxTokenExpiration?
   Yes --> error: "requested expiration duration exceeds max"
   No  --> continue

3. Is the caller a provider-tenant member?
   Yes --> resolve their admin role (EDITOR or VIEWER based on provider-tenant membership)
   No  --> no admin role assigned

4. If admin role is requested:
   - Is caller allowed to have the requested admin role?
     No  --> error: "not member of provider tenant"
     Yes --> continue

5. validateTokenRequest (permission elevation prevention):
   - Are all requested tenant IDs either "*" or allowed for the caller?
     No  --> error: "requested tenant roles are not allowed"
   - Are all requested tenant role types valid (not UNSPECIFIED)?
     No  --> error: "requested tenant role is not allowed"
   - Are all requested project IDs either "*" or allowed for the caller?
     No  --> error: "requested project roles are not allowed"
   - Are all requested project role types valid?
     No  --> error: "requested project role is not allowed"
   - Do all custom MethodPermission methods exist?
     No  --> error: "unknown method X"
   - Are all requested machine UUIDs either "*" or allowed for the caller?
     No  --> error: "requested machine roles are not allowed"

6. For each method in the requested permission set:
   - Does the caller already have permission to this method?
     No  --> error: "the following method is not allowed"
   - If the method is subject-scoped:
     Does the caller have authority over the requested subject?
     No  --> error: "method X is not allowed on subject Y"
     Yes --> proceed

7. Create JWT with all roles and permissions, sign with latest private key
```

## Appendix B: Token Update Rules

Token updates follow the same elevation-prevention logic as creation. Additionally:

- Only tokens of type `TOKEN_TYPE_API` can be updated (not user tokens).
- The update requires the calling token to possess permissions that cover all permissions on the updated token.
- Users cannot grant permissions to a token that they themselves do not possess.

## Appendix C: Token Refresh Rules

- Refresh re-issues the token with the same permissions.
- The new token's expiration is calculated as: `original_expiry - original_issued_at` (preserving the original duration).
- All role assignments from the old token are copied to the new token.
- The caller's current permissions (not the old token's) are validated against the requested permissions (which are the same as the old token's, so this is effectively a verification step).
