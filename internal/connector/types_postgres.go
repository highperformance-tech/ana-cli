package connector

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
