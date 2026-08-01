package main

import (
	"net"
	"testing"
)

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
