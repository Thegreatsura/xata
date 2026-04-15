package idgen

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
)

func TestIDs(t *testing.T) {
	for name, test := range map[string]struct {
		makeID   func() string
		sortable bool
		prefix   string
		suffix   string
		check    func(string) error
	}{
		"Generate": {
			makeID: Generate,
		},
		"GenerateWithPrefix": {
			makeID: func() string { return GenerateWithPrefix("test") },
			prefix: "test",
		},
		"GenerateSortable": {
			makeID:   GenerateSortable,
			sortable: true,
		},
		"GenerateSortableWithPrefix": {
			makeID:   func() string { return GenerateSortableWithPrefix("test") },
			sortable: true,
			prefix:   "test",
		},
		"GenerateClusterID": {
			makeID: GenerateClusterID,
			check: func(id string) error {
				if unicode.IsNumber(rune(id[0])) {
					return fmt.Errorf("malformed DNS")
				}
				return nil
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			id := test.makeID()
			idHash := id
			if test.prefix != "" {
				idPrefix := test.prefix + "_"
				assert.True(t, strings.HasPrefix(id, idPrefix))
				idHash = idHash[len(idPrefix):]
			}
			if test.suffix != "" {
				idSuffix := "_" + test.suffix
				assert.True(t, strings.HasSuffix(id, idSuffix))
				idHash = idHash[:len(idHash)-len(idSuffix)]
			}

			if test.sortable {
				id2 := test.makeID()
				assert.Greater(t, id2, id)
			}

			if test.check != nil {
				assert.NoError(t, test.check(id))
			}

			for _, r := range idHash {
				assert.True(t, ('0' <= r && r <= '9') || ('a' <= r && r <= 'v'))
			}
		})
	}
}
