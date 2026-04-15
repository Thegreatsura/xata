// Package idgen provides functions for generating IDs used through xata.
//
// All generates IDs are base32hex encoded (See [RFC 4648](https://www.rfc-editor.org/rfc/rfc4648.html)).
// The encoding uses lower case characters only, to ensure maximum compatibility between different systems.
// The only special symbol in generated IDs is `_`.
package idgen

import (
	"encoding/base32"
	"fmt"
	"unicode"

	"github.com/google/uuid"
	"github.com/rs/xid"
)

// LowerHexEncoding uses base32hex encoding. The hex encoding ensures that bitwise comparisons keep
// the ordering of the original id intact.
var LowerHexEncoding = base32.NewEncoding("0123456789abcdefghijklmnopqrstuv").WithPadding(base32.NoPadding)

const (
	PrefixMigration        = "mig"
	PrefixMigrationJob     = "mig_job"
	PrefixLog              = "log"
	PrefixMigrationRequest = "mr"
	PrefixUser             = "usr"
)

// Generate creates a new ID.
// The generated ID is base32hex encoded and contains only the following characters:
// 0123456789abcdefghijklmnopqrstuv
func Generate() string {
	id := uuid.New()
	return LowerHexEncoding.EncodeToString(id[:])
}

// GenerateClusterID creates a new ID that is compliant with DNS format.
func GenerateClusterID() string {
	id := Generate()
	// If the ID starts with a number, it is not a valid DNS name.
	if unicode.IsDigit(rune(id[0])) {
		return GenerateClusterID()
	}

	return id
}

// GenerateWithPrefix creates a new ID separating the prefix and the generated
// ID with an underscore character like `<prefix>_<id>`.
// See: Generate() for more details about the generated ID
func GenerateWithPrefix(prefix string) string {
	if prefix == "" {
		return Generate()
	}
	return fmt.Sprintf("%s_%s", prefix, Generate())
}

// GenerateSortable generates a base32hex encoded ID. IDs are sortable by the time
// GenerateSortable has been used.
func GenerateSortable() string {
	return xid.New().String()
}

// GenerateSortableWithPrefix creates a new ID separating the prefix and the generated ID with an underscore character.
// See: GenerateSortable() for more details about the generated ID
func GenerateSortableWithPrefix(prefix string) string {
	if prefix == "" {
		return GenerateSortable()
	}
	return fmt.Sprintf("%s_%s", prefix, GenerateSortable())
}

// GenerateOrganizationID generates an ID for Organization.
// The ID is not guaranteed to be sortable.
func GenerateOrganizationID() string {
	return Generate()[0:6]
}
