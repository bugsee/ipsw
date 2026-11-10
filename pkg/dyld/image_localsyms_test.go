package dyld

import (
	"slices"
	"testing"

	"github.com/blacktop/go-macho/types"
)

func localSyms(entries ...struct {
	name  string
	value uint64
}) []*CacheLocalSymbol64 {
	out := make([]*CacheLocalSymbol64, 0, len(entries))
	for _, e := range entries {
		out = append(out, &CacheLocalSymbol64{
			Name: e.name,
			Nlist64: types.Nlist64{
				Nlist: types.Nlist{Type: 0x0e, Sect: 1},
				Value: e.value,
			},
		})
	}
	return out
}

func names(syms []*CacheLocalSymbol64) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}

// Identical-code-folded functions share one address, and symbol consumers (for example
// symbolic) keep the first symbol at an address. The cache's nlist order therefore has to
// survive into LocalSymbols, which is what dyld extract writes into the extracted symtab.
func TestLocalSymbolsKeepCacheOrder(t *testing.T) {
	type e = struct {
		name  string
		value uint64
	}
	img := &CacheImage{Name: "/usr/lib/libfoo.dylib"}
	img.LocalSymbols = localSyms(
		e{"_zeta_alias", 0x1000}, // same address as _alpha_alias, but first in the cache
		e{"_alpha_alias", 0x1000},
		e{"_mid", 0x2000},
		e{"_beta", 0x3000},
	)
	want := []string{"_zeta_alias", "_alpha_alias", "_mid", "_beta"}

	img.indexLocalSymbols()

	if got := names(img.LocalSymbols); !slices.Equal(got, want) {
		t.Fatalf("LocalSymbols was reordered: got %v, want %v", got, want)
	}
	if got := names(img.localsByName); !slices.IsSorted(got) {
		t.Fatalf("localsByName is not sorted by name: %v", got)
	}

	macho := img.GetLocalSymbolsAsMachoSymbols()
	if len(macho) != len(want) {
		t.Fatalf("got %d macho symbols, want %d", len(macho), len(want))
	}
	for idx, s := range macho {
		if s.Name != want[idx] {
			t.Errorf("macho symbol %d = %q, want %q (cache order)", idx, s.Name, want[idx])
		}
	}
}

func TestGetLocalSymbolFindsByName(t *testing.T) {
	type e = struct {
		name  string
		value uint64
	}
	img := &CacheImage{Name: "/usr/lib/libfoo.dylib"}
	img.LocalSymbols = localSyms(e{"_zeta", 0x1000}, e{"_alpha", 0x2000}, e{"_mid", 0x3000})
	img.indexLocalSymbols()
	img.Analysis.State.SetPrivates(true) // skip parsing from a (non-existent) cache

	for _, tc := range []struct {
		name string
		want uint64
	}{{"_zeta", 0x1000}, {"_alpha", 0x2000}, {"_mid", 0x3000}} {
		sym, err := img.GetLocalSymbol(tc.name)
		if err != nil {
			t.Fatalf("GetLocalSymbol(%q): %v", tc.name, err)
		}
		if sym.Value != tc.want {
			t.Errorf("GetLocalSymbol(%q).Value = %#x, want %#x", tc.name, sym.Value, tc.want)
		}
	}
	if _, err := img.GetLocalSymbol("_missing"); err == nil {
		t.Error("GetLocalSymbol(_missing) succeeded, want an error")
	}
}

// With duplicate names the first one in the cache's order wins, deterministically.
func TestGetLocalSymbolDuplicateNamesUseCacheOrder(t *testing.T) {
	type e = struct {
		name  string
		value uint64
	}
	img := &CacheImage{Name: "/usr/lib/libfoo.dylib"}
	img.LocalSymbols = localSyms(e{"_dup", 0x2000}, e{"_other", 0x1000}, e{"_dup", 0x3000})
	img.indexLocalSymbols()
	img.Analysis.State.SetPrivates(true)

	sym, err := img.GetLocalSymbol("_dup")
	if err != nil {
		t.Fatal(err)
	}
	if sym.Value != 0x2000 {
		t.Errorf("GetLocalSymbol(_dup).Value = %#x, want 0x2000 (first in cache order)", sym.Value)
	}
}
