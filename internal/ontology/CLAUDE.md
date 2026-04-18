# internal/ontology

The `ana ontology` verb tree: `list`, `get`. Readonly surface. Dispatch-only around `Deps.Unary`.

## Files

- `ontology.go` — `New`, `Deps`, service path prefix (`OntologyService`).
- `list.go` — `GetOntologies` / `GetOntologiesSummary`.
- `get.go` — `GetOntologyById`.
- `ontology_test.go` — fake `Unary` covers each subcommand.
