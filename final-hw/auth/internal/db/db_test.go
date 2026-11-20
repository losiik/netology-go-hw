package db

import "testing"

func TestDSNFromEnv(t *testing.T) {
	keys := []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASS", "DB_NAME"}

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			"defaults",
			map[string]string{},
			"postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable",
		},
		{
			"database_url_priority",
			map[string]string{"DATABASE_URL": "postgres://u:p@db:1/x", "DB_HOST": "ignored"},
			"postgres://u:p@db:1/x",
		},
		{
			"from_parts",
			map[string]string{"DB_HOST": "pg", "DB_PORT": "5433", "DB_USER": "app", "DB_PASS": "p@ss", "DB_NAME": "money"},
			"postgres://app:p%40ss@pg:5433/money?sslmode=disable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range keys {
				t.Setenv(k, tc.env[k])
			}
			if got := DSNFromEnv(); got != tc.want {
				t.Errorf("DSNFromEnv() = %q, ожидали %q", got, tc.want)
			}
		})
	}
}

func TestSafeDSN(t *testing.T) {
	got := SafeDSN("postgres://user:secret@localhost:5432/db")
	if got != "postgres://user:xxxxx@localhost:5432/db" {
		t.Errorf("SafeDSN() = %q", got)
	}
}
