package identity

const (
	ClientCredential    Prefix = "ccd"
	WebhookSubscription Prefix = "whs"
	WebhookDelivery     Prefix = "whd"
)

func init() {
	registeredPrefixes = append(registeredPrefixes, ClientCredential, WebhookSubscription, WebhookDelivery)
}

type clientCredentialKind struct{}

func (clientCredentialKind) resourcePrefix() Prefix { return ClientCredential }

type ClientCredentialID = TypedID[clientCredentialKind]

func NewClientCredentialID() (ClientCredentialID, error) { return newTypedID[clientCredentialKind]() }
func ParseClientCredentialID(value string) (ClientCredentialID, error) {
	return parseTypedID[clientCredentialKind](value)
}
func ClientCredentialIDFromUUIDBytes(value [16]byte) (ClientCredentialID, error) {
	return typedIDFromUUIDBytes[clientCredentialKind](value)
}
