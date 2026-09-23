package beeorm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCacheKeySearch(t *testing.T) {
	schema := &tableSchema{cachePrefix: "abcde"}

	assert.Equal(t, "abcde_CachedIndexPlayerSkin:5:49914:5:10235", getCacheKeySearch(schema, "CachedIndexPlayerSkin", uint32(49914), uint32(10235)))
	assert.Equal(t, "abcde_CachedIndexAll", getCacheKeySearch(schema, "CachedIndexAll"))

	// production case: both pairs had the same 32-bit FNV-1a hash of "[player skin]"
	assert.NotEqual(t,
		getCacheKeySearch(schema, "CachedIndexPlayerSkin", uint32(49914), uint32(10235)),
		getCacheKeySearch(schema, "CachedIndexPlayerSkin", uint32(5674670), uint32(9592)))

	// separator inside a string parameter must not be ambiguous
	assert.NotEqual(t,
		getCacheKeySearch(schema, "CachedIndexKey", "a:1:b", "c"),
		getCacheKeySearch(schema, "CachedIndexKey", "a", "1:b:c"))
	assert.NotEqual(t,
		getCacheKeySearch(schema, "CachedIndexKey", "12", "3"),
		getCacheKeySearch(schema, "CachedIndexKey", "1", "23"))

	// flusher builds invalidation keys from Bind (strings), searches pass typed values - both must match
	assert.Equal(t,
		getCacheKeySearch(schema, "CachedIndexPlayerSkin", uint32(49914), uint32(10235)),
		getCacheKeySearch(schema, "CachedIndexPlayerSkin", "49914", "10235"))
	assert.Equal(t,
		getCacheKeySearch(schema, "CachedIndexZone", "Europe", uint16(54)),
		getCacheKeySearch(schema, "CachedIndexZone", "Europe", "54"))
}
