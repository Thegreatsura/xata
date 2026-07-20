// Package passwords generates PostgreSQL role passwords.
package passwords

import "github.com/sethvargo/go-password/password"

// Generate generates a role password using the same parameters as CNPG
// (1.28): 64 characters with 10 digits, no symbols, mixed case, with
// repeated characters allowed.
func Generate() (string, error) {
	return password.Generate(64, 10, 0, false, true)
}
