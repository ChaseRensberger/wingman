package main

import (
	"net"
	"testing"
)

func TestServerCredentialsGenerateAndReuseServiceConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("WINGMAN_USERNAME", "")
	t.Setenv("WINGMAN_PASSWORD", "")
	firstUsername, firstPassword, err := serverCredentials()
	if err != nil {
		t.Fatal(err)
	}
	secondUsername, secondPassword, err := serverCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if firstUsername != "wingman" || firstPassword == "" || firstPassword != secondPassword || secondUsername != firstUsername {
		t.Fatalf("credentials were not generated and reused: %q %q, %q %q", firstUsername, firstPassword, secondUsername, secondPassword)
	}
}

func TestServerCredentialsUseEnvironment(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("WINGMAN_USERNAME", "operator")
	t.Setenv("WINGMAN_PASSWORD", "secret")
	username, password, err := serverCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if username != "operator" || password != "secret" {
		t.Fatalf("credentials = %q %q", username, password)
	}
}

func TestListenerURL(t *testing.T) {
	address := &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 4242}
	tests := []struct {
		host string
		want string
	}{
		{host: "127.0.0.1", want: "http://127.0.0.1:4242"},
		{host: "0.0.0.0", want: "http://127.0.0.1:4242"},
		{host: "::", want: "http://[::1]:4242"},
		{host: "daemon.example", want: "http://daemon.example:4242"},
	}
	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			if got := listenerURL(test.host, address); got != test.want {
				t.Fatalf("listenerURL() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLoopbackHost(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1", "[::1]", "localhost"} {
		if !isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = false", host)
		}
	}
	for _, host := range []string{"", "0.0.0.0", "::", "192.0.2.1", "example.test"} {
		if isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = true", host)
		}
	}
}
