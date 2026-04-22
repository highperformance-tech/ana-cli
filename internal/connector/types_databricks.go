package connector

// databricksSpec matches the oneof leaf for the DATABRICKS dialect. Host/
// httpPath/port/catalog/schema are required by the UI across every auth mode;
// the four auth modes differ only in which `databricksAuth` variant is
// populated (and in the envelope's top-level `authStrategy` for the OAuth
// split). Every field uses omitempty so each leaf emits only the fields its
// mode actually uses. Wire name asymmetry vs response side: request key is
// `databricks`, persisted response uses `databricksMetadata`.
type databricksSpec struct {
	Host     string              `json:"host,omitempty"`
	HTTPPath string              `json:"httpPath,omitempty"`
	Port     int                 `json:"port,omitempty"`
	Catalog  string              `json:"catalog,omitempty"`
	Schema   string              `json:"schema,omitempty"`
	Auth     *databricksAuthSpec `json:"databricksAuth,omitempty"`
}

// databricksAuthSpec is the nested oneof discriminator on
// `databricks.databricksAuth`. Exactly one of `Pat`, `ClientCredentials`,
// `OAuthU2M` is populated per request. Wire label mismatch note: OAuth (SSO)
// and OAuth (Individual) both populate `OAuthU2M` — `oauthU2m`, NOT
// `oauthSso`, is the only OAuth variant the server accepts. The SSO vs
// Individual split lives on the envelope's `authStrategy` (oauth_sso vs
// per_member_oauth), not on this nested oneof.
type databricksAuthSpec struct {
	Pat               *databricksPatAuth               `json:"pat,omitempty"`
	ClientCredentials *databricksClientCredentialsAuth `json:"clientCredentials,omitempty"`
	OAuthU2M          *databricksOAuthU2MAuth          `json:"oauthU2m,omitempty"`
}

// databricksPatAuth carries the Access Token (Personal Access Token) variant.
// Single field `token` — the server never echoes it back on GetConnector.
type databricksPatAuth struct {
	Token string `json:"token,omitempty"`
}

// databricksClientCredentialsAuth is the M2M Service-Principal variant.
// `clientId` is a non-sensitive UUID (the SP's applicationId) and IS echoed
// on GetConnector; `clientSecret` is the server-side secret and is absent
// from the persisted echo.
type databricksClientCredentialsAuth struct {
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// databricksOAuthU2MAuth is the U2M (user-to-machine) variant — same struct
// serves both `oauth-sso` (shared refresh token) and `oauth-individual`
// (per-member lazy OAuth); the envelope's `authStrategy` is the only thing
// that differs. `clientId` is echoed on GetConnector; `clientSecret` is not.
type databricksOAuthU2MAuth struct {
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
}
