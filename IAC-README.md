# IaC reader (state file) for GitHub – file list and steps

This guide walks through **only the IaC reader path**: how driftctl reads a Terraform state file and turns it into a list of resources. Focus is on **GitHub** resources (`github_*`).

---

## 1. Files to read (in order)

Read these in sequence so the flow makes sense.

### Config and “from” parsing

| # | File | What to look at |
|---|------|------------------|
| 1 | `pkg/iac/config/config.go` | `SupplierConfig` (Key, Backend, Path). Used for `tfstate` + backend + path. |
| 2 | `pkg/cmd/flags.go` | `parseFromFlag()` – how `--from tfstate://path` becomes `SupplierConfig`. |
| 3 | `pkg/iac/supplier/supplier.go` | `GetIACSupplier()` – builds the IaC supplier from config; for `tfstate` it creates `TerraformStateReader`. |

### Backend (where state bytes come from)

| # | File | What to look at |
|---|------|------------------|
| 4 | `pkg/iac/terraform/state/backend/backend.go` | `Backend` type (`io.ReadCloser`), `GetBackend()`, `BackendKeyFile` (empty = local file). |
| 5 | `pkg/iac/terraform/state/backend/file_reader.go` | `NewFileReader(path)` – opens local state file. |

### State parsing and version check

| # | File | What to look at |
|---|------|------------------|
| 6 | `pkg/iac/terraform/state/versions.go` | `IsVersionSupported()` – state Terraform version must be ≥ 0.11. |
| 7 | `pkg/iac/terraform/state/terraform_state_reader.go` | `readState()` (uses `statefile.Read`), `read()`, then `retrieve()` → `decode()` → `Resources()`. This is the core. |

### Provider and schema (needed to decode state)

| # | File | What to look at |
|---|------|------------------|
| 8 | `enumeration/terraform/providers.go` | `ProviderLibrary`, `AddProvider`, `Provider(name)` – holds GitHub provider. |
| 9 | `enumeration/terraform/terraform_provider.go` | `TerraformProvider` interface – `Schema()` is used to decode state. |
| 10 | `enumeration/remote/github/init.go` | `Init()` – adds GitHub provider to library via `providerLibrary.AddProvider(terraform.GITHUB, provider)`. |
| 11 | `enumeration/remote/github/provider.go` | `NewGithubTerraformProvider()` – builds the provider (starts Terraform provider binary for schema). |

### Resource types and deserialization

| # | File | What to look at |
|---|------|------------------|
| 12 | `pkg/resource/resource_types.go` | `IsResourceTypeSupported()` – `github_*` types are supported (around line 182–186). |
| 13 | `pkg/resource/deserializer.go` | `DeserializeOne(ty, cty.Value)` – state value → `*resource.Resource` (id, type, attrs). |
| 14 | `pkg/resource/factory.go` | `CreateAbstractResource(ty, id, attrs)` – builds the `Resource` struct. |
| 15 | `pkg/resource/schemas/repository.go` | `Init()` – loads provider schema and calls `github.InitResourcesMetadata(r)` for GitHub. |
| 16 | `pkg/resource/github/metadatas.go` | GitHub resource type constants and metadata. |

### Optional (chain and interface)

| # | File | What to look at |
|---|------|------------------|
| 17 | `pkg/iac/supplier/IacChainSupplier.go` | How multiple state sources are merged when you pass several `--from`. |
| 18 | `pkg/resource/supplier.go` | `IaCSupplier` interface – `Resources()`, `SourceCount()`. |

---

## 2. Steps to follow (conceptual)

1. **Parse “from”**  
   Turn `--from tfstate://./terraform.tfstate` (or `tfstate+file://...`) into `SupplierConfig{Key: "tfstate", Backend: "file" or "", Path: "..."}`.

2. **Resolve backend**  
   For local file: `Backend = ""` → `GetBackend()` → `NewFileReader(config.Path)` → you get an `io.ReadCloser` over the state file.

3. **Read state**  
   `statefile.Read(reader)` (Hashi’s state format) → `*statefile.File` → take `State` → `*states.State` (modules, resources, instances).

4. **Version check**  
   `IsVersionSupported(state.TerraformVersion)` → must be ≥ 0.11.

5. **Walk state and filter**  
   For each module and each resource in state:
   - Skip if `!IsResourceTypeSupported(resType)` (e.g. only `github_*` and other supported types pass).
   - Skip if filter ignores the type.
   - Skip if not a managed resource.
   - Resolve provider: `providerLibrary.Provider(providerType)` (e.g. `"github"`) – **must be non-nil** (so GitHub provider must be activated to get schema).
   - Get schema: `provider.Schema()[stateRes.Addr.Resource.Type]`.
   - For each instance: decode with `instance.Current.Decode(schema.Block.ImpliedType())` (or `convertInstance` on path error).
   - Collect decoded cty values keyed by resource type.

6. **Decode to resources**  
   For each (type, cty value): `deserializer.DeserializeOne(ty, val)` → JSON-marshal cty, unmarshal to `Attributes`, then `factory.CreateAbstractResource(ty, id, attrs)` → `*resource.Resource`. Set `Source` (e.g. state path, module, name).

7. **Return only GitHub**  
   After `Resources()`, keep only resources where `strings.HasPrefix(res.Type, "github_")` (or use the same type set as in `resource_types.go`).

---

## 3. Minimal experiment: “get all GitHub resources from state”

- Use the **existing** driftctl IaC stack; don’t reimplement state parsing.
- Ensure the **GitHub Terraform provider is activated** so the provider library has the GitHub provider and its schema (needed in step 5).
- Build the same pipeline: config → `GetIACSupplier()` → `iacSupplier.Resources()`.
- Then filter the returned slice to `github_*` and print or process them.

The runnable example that does this is in **`tools/iac-reader-github/`**. Run from repo root:

```bash
# From driftctl repo root, with a terraform.tfstate that contains github_* resources
go run ./tools/iac-reader-github/ --from tfstate://./terraform.tfstate
```

It will:

1. Activate the GitHub provider (so schema is available).
2. Build the IaC supplier from `--from`.
3. Call `Resources()` (read state, decode with provider schema, deserialize).
4. Print only resources whose type starts with `github_`.

You can then adapt this to write your own code that “replicates” only the IaC reader part for GitHub.
