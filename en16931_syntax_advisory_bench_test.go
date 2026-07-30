package formalis

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// What the advisory bindings cost, in two halves: the pass itself, and the share
// of a whole validation it accounts for.
//
// The reason to have this committed rather than to have measured it once is that
// 1,168 rules is the kind of number that invites a naive implementation — one
// tree traversal per rule — and the difference between that and one gathering
// walk is three orders of magnitude, not a percentage. A benchmark makes the
// claim in en16931_syntax_advisory_eval.go's comment reproducible by anyone who
// doubts it.
//
// The corpus is the gitignored conformance corpus, so these skip on a bare
// checkout rather than reporting a number backed by nothing.

var benchDocs = sync.OnceValue(func() map[string][][]byte {
	out := map[string][][]byte{}
	_ = filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".xml" {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		root, perr := parseCII(newRun(context.Background()), data)
		if perr != nil || root == nil {
			return nil
		}
		switch root.name {
		case "Invoice", "CreditNote":
			if len(out["UBL"]) < 200 {
				out["UBL"] = append(out["UBL"], data)
			}
		case "CrossIndustryInvoice":
			if len(out["CII"]) < 200 {
				out["CII"] = append(out["CII"], data)
			}
		}
		return nil
	})
	return out
})

func benchCorpus(b *testing.B, syntax string) [][]byte {
	b.Helper()
	docs := benchDocs()[syntax]
	if len(docs) == 0 {
		b.Skipf("no %s documents present; run `make cius-oracles en16931-ubl`", syntax)
	}
	return docs
}

// BenchmarkAdvisorySyntaxRules is the pass this PR added, over an already-parsed
// tree: the single gathering walk, the first-match assignment, and the assertion
// evaluation, with no parsing and no semantic model in the number.
func BenchmarkAdvisorySyntaxRules(b *testing.B) {
	for _, syntax := range []string{"UBL", "CII"} {
		b.Run(syntax, func(b *testing.B) {
			var trees []*ciiNode
			for _, d := range benchCorpus(b, syntax) {
				root, err := parseCII(newRun(context.Background()), d)
				if err != nil {
					b.Fatal(err)
				}
				trees = append(trees, root)
			}
			r := newRun(context.Background())
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, root := range trees {
					advisorySyntaxRules(r, root)
				}
			}
			b.ReportMetric(float64(len(trees)), "docs/op")
		})
	}
}

// BenchmarkValidateEN16931 is the whole call a caller makes, so the pass above
// can be read as a share of it rather than in isolation.
func BenchmarkValidateEN16931(b *testing.B) {
	for _, syntax := range []string{"UBL", "CII"} {
		b.Run(syntax, func(b *testing.B) {
			docs := benchCorpus(b, syntax)
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, d := range docs {
					if _, err := Validate(ctx, d, ProfileEN16931); err != nil {
						b.Fatal(err)
					}
				}
			}
			b.ReportMetric(float64(len(docs)), "docs/op")
		})
	}
}
