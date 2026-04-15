package idgen

// SymbolIDGenerator creates a sequence of sortable IDs.
// The IDs are based on lower case characters a-z only.
// IDs are generated sequential like
// `a, b, ... z, aa, ab, ... az, ba, bb, ... bz, ...`
// and so on.
type SymbolIDGenerator struct {
	value string
}

// SymbolIDGeneratorFrom creates a new generator from the given seed.
// The next ID will be generated from the seed value.
func SymbolIDGeneratorFrom(seed string) SymbolIDGenerator {
	return SymbolIDGenerator{value: seed}
}

// Status returns the current ID status. The status can be used with
// ShortIDGeneratorFrom to create a new generator that continues from the
// current status.
func (gen *SymbolIDGenerator) Status() string {
	return gen.value
}

// Next creates a new ID.
func (gen *SymbolIDGenerator) Next() string {
	gen.value = getNextID(gen.value)
	return gen.value
}

func getNextID(lastID string) string {
	if len(lastID) == 0 {
		return "a"
	}
	lastChar := lastID[len(lastID)-1]
	if lastChar == 'z' {
		return getNextID(lastID[:len(lastID)-1]) + "a"
	}
	return lastID[:len(lastID)-1] + string(lastChar+1)
}
