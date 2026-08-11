package domain

// SystemInfo is the transport-independent public build identity.
type SystemInfo struct {
	APIVersion    string
	SchemaVersion string
	BuildVersion  string
}
