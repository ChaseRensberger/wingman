package route

import "fmt"

type bearerAuth struct {
	token string
}

type headerAuth struct {
	name  string
	value string
}

// Bearer returns auth that sets Authorization: Bearer <token>.
func Bearer(token string) Auth { return bearerAuth{token: token} }

// Header returns auth that sets a provider-specific auth header.
func Header(name, value string) Auth { return headerAuth{name: name, value: value} }

func (a bearerAuth) Apply(req *PreparedRequest) error {
	if a.token == "" {
		return fmt.Errorf("route auth: missing bearer token")
	}
	req.Headers.Set("Authorization", "Bearer "+a.token)
	return nil
}

func (a headerAuth) Apply(req *PreparedRequest) error {
	if a.name == "" {
		return fmt.Errorf("route auth: missing header name")
	}
	if a.value == "" {
		return fmt.Errorf("route auth: missing %s value", a.name)
	}
	req.Headers.Set(a.name, a.value)
	return nil
}
