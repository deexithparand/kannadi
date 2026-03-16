- pkg/driftctl.go

```
type DriftCTL struct {
	remoteSupplier           resource.Supplier
	iacSupplier              dctlresource.IaCSupplier
	alerter                  alerter.AlerterInterface
	analyzer                 *analyser.Analyzer
	resourceFactory          resource.ResourceFactory
	scanProgress             globaloutput.Progress
	iacProgress              globaloutput.Progress
	resourceSchemaRepository dctlresource.SchemaRepositoryInterface
	opts                     *ScanOptions
	store                    memstore.Store
}
```

- checkout the remoteSupplier and iacSupplier
