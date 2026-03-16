Here’s a concise map of the driftctl scanning pipeline and how to reuse the ideas in your own tool.

---

# Driftctl scanning pipeline – flow and core parts

Your list is right; the only addition is **middlewares** and the **unified resource model**. Below is the refined set and how they connect.

---

## Core components (refined list)

| # | Component | Your name | In driftctl |
|---|-----------|------------|-------------|
| 1 | **Cloud resource enumerator** | Azure resource enumerator | Scanner + per-type Enumerators (Azure, AWS, GCP, …) |
| 2 | **Terraform state reader** | Terraform state reader | TerraformStateReader + backends (file, S3, Azure, …) |
| 3 | **Diff engine** | Diff engine | `Analyzer.Analyze()` (match by type+id, classify managed/deleted/unmanaged) |
| 4 | **Normalization layer** | Normalization layer | `CreateAbstractResource` (schema, defaults, NormalizeFunc) + **middlewares** |
| 5 | **Middlewares** | *(you had not listed)* | Chain that aligns state vs cloud representation before diff |
| 6 | **Unified resource model** | *(implicit)* | Single `*resource.Resource` (id, type, attrs) for both state and cloud |

So: **enumerator + state reader + normalization (+ middlewares) + diff engine**, all speaking the same **resource** type.

---

## End-to-end flow (as in the codebase)

```
main.go
  → Cobra root (driftctl.go)
    → scan command (scan.go): parse --from, --to, build suppliers
      → DriftCTL.Run() (driftctl.go)
```

Inside **`Run()`**:

1. **Dual fetch – `scan()`**
   - **IaC:** `d.iacSupplier.Resources()` → list of resources from Terraform state (and optionally other IaC sources).
   - **Cloud:** `d.remoteSupplier.Resources()` → Scanner runs all enumerators in parallel; merged list = “what’s in the cloud”.
2. **Normalize remote list**
   - Each cloud resource is passed through `d.resourceFactory.CreateAbstractResource(ty, id, attrs)` so both sides use the same abstraction (and schema normalization).
3. **Middlewares**
   - `middleware.Execute(&remoteResources, &resourcesFromState)` runs a fixed chain (Route53, S3, VPC, IAM, Azure route/subnet, etc.) that can add/remove/transform resources so state and cloud are comparable (e.g. expand IAM attachments, drop defaults).
4. **Optional filter**
   - If `--filter` is set, JMESPath is applied to both slices.
5. **Analysis (diff)**
   - `d.analyzer.Analyze(remoteResources, resourcesFromState)`:
     - For each **state** resource, find a “corresponding” **remote** resource by `res.Equal(r)` (same type + id, optional DiscriminantFunc).
     - No match for state → **deleted** (in state, not in cloud).
     - Match found → **managed**; remove that remote from the list.
     - Remaining remote → **unmanaged** (in cloud, not in state).
6. **Output**
   - Analysis (managed / unmanaged / deleted + summary) is written by the chosen output (console, JSON, etc.).

So: **IaC state + cloud enumeration → normalize remote → middlewares → optional filter → analyzer (match + classify) → output.**

---

## 1. Azure (cloud) resource enumerator

**Where:** `enumeration/remote/` (and Azure-specific: `enumeration/remote/azurerm/`).

- **Activation:** `enumeration/remote/remote.go` → `Activate(to, ...)` calls e.g. `azurerm.Init(...)` when `to` is Azure.
- **Init** (`enumeration/remote/azurerm/init.go`):
  - Creates Terraform provider and Azure clients (DefaultAzureCredential, ARM client options).
  - Builds **repositories** (API wrappers): e.g. `StorageRepository`, `NetworkRepository`, `ResourcesRepository`, etc. in `enumeration/remote/azurerm/repository/`.
  - Registers one **enumerator** per resource type with `remoteLibrary.AddEnumerator(...)` (e.g. storage account, container, vnet, route table, subnet, NSG, load balancer, Private DNS, compute, …).

**Enumerator contract** (`enumeration/remote/common/library.go`):

- Interface: `SupportedType() resource.ResourceType`, `Enumerate() ([]*resource.Resource, error)`.
- **Scanner** (`enumeration/remote/scanner.go`): implements `resource.Supplier`; `Resources()` runs all `remoteLibrary.Enumerators()` in parallel (e.g. concurrency 10), collects results, returns one slice of `*resource.Resource`.

**Example – Azure Storage Account** (`enumeration/remote/azurerm/azurerm_storage_account_enumerator.go`):

- `SupportedType()` → `AzureStorageAccountResourceType`.
- `Enumerate()` → `e.repository.ListAllStorageAccount()` then, for each account, `e.factory.CreateAbstractResource(type, *account.ID, map[string]interface{}{})`.

So for “build Azure resource enumerator”: implement **repositories** (call Azure APIs), then **one enumerator per resource type** that returns `[]*resource.Resource`; register them in a “remote library” and have a **Scanner** run them in parallel and merge.

---

## 2. Terraform state reader

**Where:** `pkg/iac/terraform/state/terraform_state_reader.go` and `pkg/iac/terraform/state/backend/`.

- **Backend resolution:** `backend.GetBackend(config, backendOptions)` returns a `Backend` (e.g. file, S3, GCS, HTTP, TF Cloud, Azure). Backend gives you an `io.ReadCloser` for the state (by path/URL).
- **Reading:** `read(path, backend)` uses Hashicorp’s `statefile.Read(reader)` to parse the state file into `*states.State`.
- **State traversal:** Iterate `state.Modules` → each module’s `Resources` → `Instances`. Decode each instance with the provider schema (`provider.Schema()[type]`, `instance.Current.Decode(schema.Block.ImpliedType())`), with fallback for provider version mismatch (cty conversion). You get decoded resources (e.g. type → `decodedRes{ source, cty.Value }`).
- **To unified resources:** A **Deserializer** turns each cty value into a `*resource.Resource`: e.g. get `id` and attributes, then `factory.CreateAbstractResource(ty, id, attrs)`. So state-side resources are also normalized the same way.

**Multiple states:** If the backend has a **state enumerator** (e.g. S3 prefix, Azure blob prefix), the reader can enumerate “state keys” and call the same read+decode+deserialize for each key, then merge all resources.

**IaC supplier** (`pkg/iac/supplier/supplier.go`): For each `--from` config (e.g. `tfstate+s3://bucket/key`), it creates a `TerraformStateReader` and adds it to an `IacChainSupplier`, which aggregates `Resources()` from all readers.

So for “build Terraform state reader”: implement **backends** (how to open state bytes), use **statefile.Read** (or equivalent) to get `states.State`, iterate modules/resources/instances, **decode with provider schema**, then **Deserializer + CreateAbstractResource** to get the same `*resource.Resource` shape as the enumerator.

---

## 3. Diff engine

**Where:** `pkg/analyser/analyzer.go` (and types in `pkg/analyser/analysis.go`).

- **Input:** Two slices: `remoteResources` (from cloud) and `resourcesFromState` (from IaC), both `[]*resource.Resource`.
- **Filtering:** Optionally drop resources that are “ignored” by filter or alerter.
- **Matching:** For each **state** resource, find a **remote** resource with `res.Equal(r)`.
  - **`Equal`** (`enumeration/resource/resource.go`): same `ResourceId()` and `ResourceType()`; if the resource type has a schema with `DiscriminantFunc`, use that; otherwise “same id + type” ⇒ equal.
- **Classification:**
  - No match for state → **deleted** (in state, not in cloud).
  - Match found → **managed**; remove that remote from the list.
  - After processing all state resources, remaining remote → **unmanaged** (in cloud, not in state).
- **Result:** `Analysis`: lists managed / unmanaged / deleted, summary counts, alerts, duration.

So the “diff” here is **set comparison by (type, id) plus optional DiscriminantFunc** — it does **not** compute per-attribute diffs; it only decides “managed / deleted / unmanaged”. For your own tool you can keep this and add attribute-level diff later if needed.

---

## 4. Normalization layer

Two parts:

**A) Abstract resource creation (single canonical shape)**  
- **Factory** (`pkg/resource/factory.go`): `CreateAbstractResource(ty, id, data)`:
  - Wraps `data` in `resource.Attributes(data)`, then `attributes.SanitizeDefaults()`.
  - Looks up schema for `ty` from the schema repository; attaches schema to the resource.
  - If the schema has `NormalizeFunc`, runs `schema.NormalizeFunc(&res)`.
- Used when:
  - Building resources **from state** (Deserializer → CreateAbstractResource).
  - Building resources **from remote** (each enumerator uses the same factory).
  - **After scan:** in `scan()`, remote resources are re-built through `CreateAbstractResource` so both sides use the same normalized shape.

**B) Middlewares (semantic alignment)**  
- **Chain** (`pkg/driftctl.go`): Route53, S3, VPC, IAM, API Gateway, Google IAM, **Azure route/subnet expander**, etc.
- **Contract:** `Execute(remoteResources, resourcesFromState *[]*resource.Resource) error` — can add/remove/transform entries in both slices so that state and cloud representations are comparable (e.g. one state resource → many cloud resources, or ignore default resources).

So “normalization” = **CreateAbstractResource (schema + defaults + NormalizeFunc)** + **middleware chain** that aligns state vs cloud before the diff.

---

## 5. Data structures (for your own design)

- **Unified resource:** `enumeration/resource/resource.go` — `Resource`: `Id`, `Type`, `Attrs *Attributes`, `Sch *Schema`, `Source Source`. Methods: `ResourceId()`, `ResourceType()`, `Equal(res)`.
- **State:** Hashicorp `states.State` (modules → resources → instances); decode instances to cty with provider schema.
- **Analysis:** Slices `managed`, `unmanaged`, `deleted` (`[]*resource.Resource`), plus summary and alerts.

Everything that comes from “state” or “cloud” is turned into this same `*resource.Resource` type so the diff engine only deals with one shape.

---

## Flow summary (for implementing your own)

1. **CLI:** One “scan” command; parse “where is IaC” (e.g. `--from`) and “which cloud” (e.g. `--to`).
2. **Two suppliers:**
   - **IaC supplier:** State reader(s) that open state (file/S3/Azure/…), parse with statefile, decode with provider schema, then Deserializer + CreateAbstractResource → `[]*resource.Resource`.
   - **Cloud supplier:** Scanner that holds one enumerator per resource type; each enumerator calls cloud APIs and returns `[]*resource.Resource` via CreateAbstractResource; run enumerators in parallel and merge.
3. **Single resource type:** One `Resource` (id, type, attributes, optional schema) for both state and cloud.
4. **Normalize:** Run all resources through CreateAbstractResource (schema + defaults + NormalizeFunc). Optionally run a middleware chain to align state vs cloud (expand/merge/ignore).
5. **Compare:** For each state resource, find a remote with same (type, id) (and optional DiscriminantFunc). State-only → deleted; matched → managed; remote-only → unmanaged. Put result in an Analysis (managed / unmanaged / deleted + summary).
6. **Output:** Consume Analysis in your desired format (CLI, JSON, etc.).

If you want to go deeper next, we can zoom into one of: only Azure enumeration (repos + enumerators), only state backends + decode path, or only the analyzer + Equal/DiscriminantFunc.