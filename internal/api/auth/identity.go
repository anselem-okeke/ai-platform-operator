package auth

type Identity struct {
	Subject           string
	PreferredUsername string
	ClientID          string
	Roles             []string
}

func (i Identity) HasRole(
	role string,
) bool {
	for _, assignedRole := range i.Roles {
		if assignedRole == role {
			return true
		}
	}

	return false
}
