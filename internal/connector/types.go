package connector

// Shared wire types for the Connect-RPC Connector service. Field names follow
// the captured API shapes in `api-catalog/`; anything else is rejected
// server-side. Kept in one file so per-dialect files only need to add their
// own `<dialect>Spec` struct and point configEnvelope at it.

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

// postgresSpec matches the oneof leaf for the POSTGRES dialect. Port is an int
// per the catalog; sslMode is a boolean named `sslMode` (not `ssl`).
type postgresSpec struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
	SSLMode  bool   `json:"sslMode,omitempty"`
}

// snowflakeSpec matches the oneof leaf for the SNOWFLAKE dialect. One struct
// covers all four auth modes (password, key-pair, oauth-sso, oauth-individual)
// — the server discriminates by which credential field is populated, paired
// with `configEnvelope.AuthStrategy`. Every field is omitempty so each leaf
// only emits the fields its mode actually uses. `locator` is TextQL's wire
// name for what Snowflake's own docs call `account`.
type snowflakeSpec struct {
	Locator              string `json:"locator,omitempty"`
	Database             string `json:"database,omitempty"`
	Warehouse            string `json:"warehouse,omitempty"`
	Schema               string `json:"schema,omitempty"`
	Role                 string `json:"role,omitempty"`
	Username             string `json:"username,omitempty"`
	Password             string `json:"password,omitempty"`
	PrivateKey           string `json:"privateKey,omitempty"`
	PrivateKeyPassphrase string `json:"privateKeyPassphrase,omitempty"`
	OAuthClientID        string `json:"oauthClientId,omitempty"`
	OAuthClientSecret    string `json:"oauthClientSecret,omitempty"`
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
