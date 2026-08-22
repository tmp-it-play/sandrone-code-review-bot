package dashboard

import "crypto/subtle"

type Credentials struct {
	Username string
	Password string
}

func (c Credentials) Matches(username string, password string) bool {
	nameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(c.Username)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(c.Password)) == 1
	return nameMatch && passwordMatch
}
