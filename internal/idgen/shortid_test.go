package idgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSymbolID_Sequence(t *testing.T) {
	var gen SymbolIDGenerator

	got := make([]string, 27*26)
	want := make([]string, 0, cap(got))

	for i := range got {
		got[i] = gen.Next()
	}

	// generate sequence a...z
	for i := range rune(26) {
		want = append(want, string('a'+i))
	}
	// generate sequence aa ... az, ba ... bz, za ... zz
	for i := range rune(26) {
		for j := range rune(26) {
			want = append(want, string('a'+i)+string('a'+j))
		}
	}

	assert.Equal(t, want, got)
}

func TestSymbolID_Continue(t *testing.T) {
	t.Run("from sample IDs", func(t *testing.T) {
		for lastID, want := range map[string]string{
			"":     "a",
			"c":    "d",
			"ccc":  "ccd",
			"z":    "aa",
			"cz":   "da",
			"zz":   "aaa",
			"zzz":  "aaaa",
			"abcz": "abda",
			"az":   "ba",
		} {
			t.Run(lastID, func(t *testing.T) {
				gen := SymbolIDGeneratorFrom(lastID)
				assert.Equal(t, want, gen.Next())
			})
		}
	})

	t.Run("generator from old status", func(t *testing.T) {
		var gen SymbolIDGenerator
		gen.Next()
		gen.Next()

		second := SymbolIDGeneratorFrom(gen.Status())
		assert.Equal(t, gen.Next(), second.Next())
	})
}
