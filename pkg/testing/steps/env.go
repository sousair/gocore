//go:build e2e

package steps

import "github.com/joho/godotenv"

func LoadEnvFile(paths ...string) {
	if len(paths) == 0 {
		paths = []string{".env.e2e", ".env.test"}
	}

	for _, path := range paths {
		if err := godotenv.Load(path); err == nil {
			return
		}
	}
}
