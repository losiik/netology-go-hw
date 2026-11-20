package config

import (
	"testing"
	"time"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "defaults",
			env:  map[string]string{"JWT_SECRET": "secret"},
			want: Config{HTTPAddr: ":8080", LedgerAddr: "localhost:9090", AuthAddr: "localhost:9091", JWTSecret: "secret", RequestTimeout: DefaultRequestTimeout},
		},
		{
			name: "custom",
			env: map[string]string{
				"JWT_SECRET": "secret", "HTTP_ADDR": ":9000", "LEDGER_ADDR": "ledger:9090",
				"AUTH_ADDR": "auth:9091", "REQUEST_TIMEOUT": "500ms",
			},
			want: Config{HTTPAddr: ":9000", LedgerAddr: "ledger:9090", AuthAddr: "auth:9091", JWTSecret: "secret", RequestTimeout: 500 * time.Millisecond},
		},
		{name: "no_secret", env: map[string]string{}, wantErr: true},
		{name: "bad_timeout", env: map[string]string{"JWT_SECRET": "s", "REQUEST_TIMEOUT": "две секунды"}, wantErr: true},
		{name: "zero_timeout", env: map[string]string{"JWT_SECRET": "s", "REQUEST_TIMEOUT": "0s"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"JWT_SECRET", "HTTP_ADDR", "LEDGER_ADDR", "AUTH_ADDR", "REQUEST_TIMEOUT"} {
				t.Setenv(k, tc.env[k])
			}
			got, err := FromEnv()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("FromEnv() = %+v, ожидали %+v", got, tc.want)
			}
		})
	}
	if DefaultRequestTimeout != 2*time.Second {
		t.Errorf("таймаут по умолчанию %s", DefaultRequestTimeout)
	}
}
