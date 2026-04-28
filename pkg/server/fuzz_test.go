package server

import (
	"net/http"
	"testing"

	"digital.vasic.http3/internal/testcert"
)

// FuzzConfigValidate fuzzes the Addr and ALPN fields of Config to ensure
// Validate never panics on adversarial input, and that inputs which should
// be invalid (empty Addr) consistently produce a non-nil error.
//
// Sixth Law falsifiability: change the empty-Addr early return in Validate
// to `return nil` and the corresponding "expected error" branch fails.
func FuzzConfigValidate(f *testing.F) {
	seeds := []struct {
		addr string
		alpn string
	}{
		{"127.0.0.1:8443", "h3"},
		{"", ""},
		{":0", "h2"},
		{":443", ""},
		{"\x00", "junk"},
		{"[::1]:0", "h3"},
	}
	for _, s := range seeds {
		f.Add(s.addr, s.alpn)
	}

	tcBase, err := testcert.Generate()
	if err != nil {
		f.Fatalf("testcert.Generate: %v", err)
	}

	f.Fuzz(func(t *testing.T, addr string, alpn string) {
		tc := tcBase.Clone()
		if alpn != "" {
			tc.NextProtos = []string{alpn}
		}
		cfg := Config{
			Addr:    addr,
			Handler: http.NewServeMux(),
			TLSConf: tc,
		}
		err := cfg.Validate()

		// Invariants: invalid inputs MUST produce an error.
		if addr == "" && err == nil {
			t.Fatalf("empty addr accepted by Validate")
		}
	})
}
