package auth

type Claims struct {
	Subject           string `json:"sub"`
	PreferredUsername string `json:"preferred_username"`
	AuthorizedParty   string `json:"azp"`

	RealmAccess RealmAccess `json:"realm_access"`
}

type RealmAccess struct {
	Roles []string `json:"roles"`
}
