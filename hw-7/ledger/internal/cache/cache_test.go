package cache

import "testing"

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		db      string
		pass    string
		want    Config
		wantErr bool
	}{
		{"defaults", "", "", "", Config{Addr: "localhost:6379"}, false},
		{"custom", "redis:6380", "3", "secret", Config{Addr: "redis:6380", DB: 3, Password: "secret"}, false},
		{"bad_db", "", "abc", "", Config{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REDIS_ADDR", tc.addr)
			t.Setenv("REDIS_DB", tc.db)
			t.Setenv("REDIS_PASSWORD", tc.pass)

			got, err := ConfigFromEnv()
			if tc.wantErr {
				if err == nil {
					t.Fatal("ожидалась ошибка")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("ConfigFromEnv() = %+v, ожидали %+v", got, tc.want)
			}
		})
	}
}
