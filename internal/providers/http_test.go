package providers

import (
	"net/http"
	"testing"
)

func TestServiceEndpointFromOpenAICompatibleBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		endpoint string
		want     string
	}{
		{
			name:     "Sub2API usage keeps v1 endpoint",
			baseURL:  "https://host.example/v1",
			endpoint: "/v1/usage",
			want:     "https://host.example/v1/usage",
		},
		{
			name:     "New API user endpoint replaces v1 prefix",
			baseURL:  "https://host.example/v1",
			endpoint: "/api/user/self",
			want:     "https://host.example/api/user/self",
		},
		{
			name:     "trailing slash",
			baseURL:  "https://host.example/v1/",
			endpoint: "api/user/self",
			want:     "https://host.example/api/user/self",
		},
		{
			name:     "deployment prefix is preserved",
			baseURL:  "https://host.example/relay/v1?ignored=yes#ignored",
			endpoint: "/v1/usage",
			want:     "https://host.example/relay/v1/usage",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := serviceEndpoint(test.baseURL, test.endpoint)
			if err != nil {
				t.Fatalf("serviceEndpoint() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("serviceEndpoint(%q, %q) = %q, want %q", test.baseURL, test.endpoint, got, test.want)
			}
		})
	}
}

func TestServiceEndpointRejectsUnsafeBaseURL(t *testing.T) {
	tests := []string{
		"",
		"host.example/v1",
		"file:///tmp/provider",
		"https://user:password@host.example/v1",
	}
	for _, baseURL := range tests {
		t.Run(baseURL, func(t *testing.T) {
			if got, err := serviceEndpoint(baseURL, "/api/user/self"); err == nil {
				t.Fatalf("serviceEndpoint(%q) = %q, want error", baseURL, got)
			}
		})
	}
}

func TestClientForProxyValidation(t *testing.T) {
	for _, proxyURL := range []string{
		"http://127.0.0.1:8080",
		"https://proxy.example.com",
		"socks5://user:pass@127.0.0.1:1080",
		"socks5h://127.0.0.1:1080",
	} {
		t.Run(proxyURL, func(t *testing.T) {
			client, cleanup, usingProxy, err := clientForProxy(proxyURL)
			if err != nil {
				t.Fatalf("clientForProxy(%q) error = %v", proxyURL, err)
			}
			defer cleanup()
			if client == nil || !usingProxy {
				t.Fatalf("clientForProxy(%q) = (%v, %v), want configured proxy client", proxyURL, client, usingProxy)
			}
		})
	}

	for _, proxyURL := range []string{"", "direct", "none", "DIRECT"} {
		t.Run("direct_"+proxyURL, func(t *testing.T) {
			client, cleanup, usingProxy, err := clientForProxy(proxyURL)
			if err != nil {
				t.Fatalf("clientForProxy(%q) error = %v", proxyURL, err)
			}
			defer cleanup()
			if client == nil || usingProxy {
				t.Fatalf("clientForProxy(%q) = (%v, %v), want direct client", proxyURL, client, usingProxy)
			}
			transport, ok := client.Transport.(*http.Transport)
			if !ok || transport.Proxy != nil {
				t.Fatalf("clientForProxy(%q) did not disable environment proxy", proxyURL)
			}
		})
	}

	t.Run("socks5h is normalized for Go transport", func(t *testing.T) {
		client, cleanup, usingProxy, err := clientForProxy("socks5h://127.0.0.1:1080")
		if err != nil {
			t.Fatalf("clientForProxy(socks5h) error = %v", err)
		}
		defer cleanup()
		transport, ok := client.Transport.(*http.Transport)
		if !ok || !usingProxy {
			t.Fatalf("clientForProxy(socks5h) = (%T, %v), want proxy transport", client.Transport, usingProxy)
		}
		request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
		if err != nil {
			t.Fatalf("http.NewRequest() error = %v", err)
		}
		proxy, err := transport.Proxy(request)
		if err != nil {
			t.Fatalf("transport.Proxy() error = %v", err)
		}
		if proxy == nil || proxy.Scheme != "socks5" {
			t.Fatalf("normalized proxy = %v, want socks5 scheme", proxy)
		}
	})

	for _, proxyURL := range []string{"file:///tmp/proxy", "socks5:///missing-host", "://bad"} {
		t.Run(proxyURL, func(t *testing.T) {
			if _, _, _, err := clientForProxy(proxyURL); err == nil {
				t.Fatalf("clientForProxy(%q) succeeded, want error", proxyURL)
			}
		})
	}
}
