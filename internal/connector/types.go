package connector

// Shared wire types for the Connect-RPC Connector service. Field names follow
// the captured API shapes in `api-catalog/`; anything else is rejected
// server-side. Per-dialect spec structs live in `types_<dialect>.go` so new
// dialects only need to add their own file and point configEnvelope at it.

// createReq mirrors the exact wire shape captured in the API catalog.
type createReq struct {
	Config configEnvelope `json:"config"`
}

// updateReq's `connectorId` MUST sit at the top level — putting it inside
// config returns 500 "could not find connector" (captured regression).
type updateReq struct {
	ConnectorID int            `json:"connectorId"`
	Config      configEnvelope `json:"config"`
}

// configEnvelope is shared by create + update. Dialect pointers (not values)
// so update can omit the block when no dialect flags were set (partial-update
// case). AuthStrategy sits at envelope level per the captured wire shape
// (`config.authStrategy`, not nested under a dialect sub-object); it's empty
// for Postgres, populated for Snowflake/Databricks.
type configEnvelope struct {
	ConnectorType string         `json:"connectorType,omitempty"`
	Name          string         `json:"name,omitempty"`
	AuthStrategy  string         `json:"authStrategy,omitempty"`
	Postgres      *postgresSpec  `json:"postgres,omitempty"`
	Snowflake     *snowflakeSpec `json:"snowflake,omitempty"`
}

// createResp is the `{connectorId, name, connectorType}` captured response.
type createResp struct {
	ConnectorID   int    `json:"connectorId"`
	Name          string `json:"name"`
	ConnectorType string `json:"connectorType"`
}

// getConnectorResp narrows the GetConnector response to the fields the update
// flow needs to merge as a baseline. PostgresMetadata carries host/port/user/
// database/sslMode (no password — the server keeps that secret).
type getConnectorResp struct {
	Connector struct {
		ConnectorType    string       `json:"connectorType"`
		Name             string       `json:"name"`
		PostgresMetadata postgresSpec `json:"postgresMetadata"`
	} `json:"connector"`
}
