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

// BenchmarkCIUSPTDatatypeRules is the CIUS-PT datatype pass over an already-parsed
// UBL tree: the single trie walk that assigns every element to the first rule of
// each pattern whose context matches it, and the evaluation of that element's
// assertions. No parsing and no semantic model is in the number.
//
// It is committed for the reason the advisory one above is. 291 assertions over
// 173 contexts is the kind of number that invites resolving each context
// independently, and 1,690 documents go through ValidateCIUSPT on every test run.
func BenchmarkCIUSPTDatatypeRules(b *testing.B) {
	var trees []*ciiNode
	for _, d := range benchCorpus(b, "UBL") {
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
			ptDTValidate(r, root, func(string, string) {})
		}
	}
	b.ReportMetric(float64(len(trees)), "docs/op")
}

// BenchmarkValidateCIUSPT is the whole call a caller makes, so the pass above can
// be read as a share of it rather than in isolation.
func BenchmarkValidateCIUSPT(b *testing.B) {
	docs := benchCorpus(b, "UBL")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, d := range docs {
			if _, err := ValidateCIUSPT(ctx, d); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportMetric(float64(len(docs)), "docs/op")
}

// BenchmarkCIUSRORules is the CIUS-RO generated pass over an already-parsed UBL
// tree: the walk that assigns every element to the first rule of ANAF's pattern
// whose context matches it, and the evaluation of that element's assertions. No
// parsing and no semantic model is in the number.
//
// It is committed beside CIUS-PT's for one reason that is specific to this rule
// set: two thirds of ANAF's contexts are XSLT *match patterns* rather than paths
// from the document element, so the walk consults a second trie at every element
// rather than only advancing the first. That is the cost this benchmark is here to
// keep visible.
func BenchmarkCIUSRORules(b *testing.B) {
	var trees []*ciiNode
	for _, d := range benchCorpus(b, "UBL") {
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
			roValidateRules(r, root, func(string, string) {})
		}
	}
	b.ReportMetric(float64(len(trees)), "docs/op")
}

// BenchmarkValidateCIUSRO is the whole call a caller makes, so the pass above can
// be read as a share of it rather than in isolation.
func BenchmarkValidateCIUSRO(b *testing.B) {
	docs := benchCorpus(b, "UBL")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, d := range docs {
			if _, err := ValidateCIUSRO(ctx, d); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportMetric(float64(len(docs)), "docs/op")
}

// BenchmarkFacturXDataModelRules is the Factur-X profile data model over an
// already-parsed CII tree: the one trie walk that assigns every element to the
// first matching rule of each pattern, and the evaluation of that element's
// assertions. No parsing and no semantic model is in the number.
//
// It is committed for the reason the two above it are, and with more cause: this
// is the largest generated tier in the package — 903 rules carrying 1,241
// assertions in EXTENDED — and the naive shape, one root-to-context descent per
// rule, is 903 walks of the document per validation rather than one. EXTENDED is
// benchmarked rather than EN 16931 because it is the largest of the five and
// therefore the worst case a caller can ask for.
func BenchmarkFacturXDataModelRules(b *testing.B) {
	for _, profile := range []Profile{ProfileMinimum, ProfileEN16931, ProfileExtended} {
		b.Run(string(profile), func(b *testing.B) {
			var trees []*ciiNode
			for _, d := range benchCorpus(b, "CII") {
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
					facturXDataModelRules(r, root, profile)
				}
			}
			b.ReportMetric(float64(len(trees)), "docs/op")
		})
	}
}
