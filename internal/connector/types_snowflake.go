package connector

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
