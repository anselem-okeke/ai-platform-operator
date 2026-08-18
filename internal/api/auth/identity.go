package auth

import "slices"

type Identity struct {
	Subject           string
	PreferredUsername string
	ClientID          string
	Roles             []string
}

func (i Identity) HasRole(
	role string,
) bool {
	return slices.Contains(i.Roles, role)
}
