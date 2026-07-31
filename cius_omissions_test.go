package formalis

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The guards on ciusCENCopyOmissions: which CEN identifiers each authority's copy of
// CEN's Schematron leaves out, and whether CEN had published them when the copy was
// taken.
//
// This is the absence half of what cius_overrides_test.go checks for differing
// conditions, and it exists for the same reason. PR 27 established that a vendored
// copy differs from CEN's current files for two quite different reasons — the
// authority edited it, or CEN moved on — and that reading the difference by eye gets
// it wrong. Absence has exactly the same two causes and the same discriminator: CEN's
// own git history, asked at the release the copy was taken from.
//
// Three questions, separated on purpose:
//
//   - is the release pin right? TestCIUSCopyOmissionsAreClassifiedFromTheArtefacts
//     re-derives it from the copy's own content.
//   - is the split right? The same test re-derives every identifier in both lists.
//   - does it *matter*? TestOmittedCENIdentifiersUnderValidateCIUSPT measures what the
//     two classes are worth across the corpus, so a change in either direction shows
//     up as a number rather than as a claim.

// ccOmissionCopy is one authority as the omission derivation reads it: its master
// Schematron (the manifest naming every pattern its validator runs), the directory
// holding its whole published rule set, and which of CEN's files each of the copied
// groups in ccCopies is a copy of.
//
// It is separate from ccCopies rather than folded into it because the two answer
// different questions. ccCopies is "which files hold this authority's copy of CEN's
// rules", which is all the condition classification needs. This adds "which CEN file
// is each of them a copy of, what else does the authority publish, and what does its
// own manifest say it runs" — and the last of those is what tells a CEN file the
// authority chose not to run from one this repository merely does not vendor.
type ccOmissionCopy struct {
	authority string
	master    string
	dist      string
	// cenFiles is the basename of the CEN file each entry of the matching ccCopies
	// pairs list is a copy of, in the same order.
	cenFiles []string
}

var ccOmissionCopies = []ccOmissionCopy{
	{
		authority: "CIUS-PT 2.1.1 (UBL)",
		master:    "testdata/cius-pt/schematron/2.1.1/urn_feap.gov.pt_CIUS-PT_2.1.1.sch",
		dist:      "testdata/cius-pt/schematron/2.1.1",
		cenFiles:  []string{"EN16931-model.sch", "EN16931-syntax.sch"},
	},
	{
		authority: "CIUS-RO 1.0.9 (UBL)",
		master:    "testdata/cius-ro/schematron/1.0.9/EN16931-CIUS_RO-UBL-validation.sch",
		dist:      "testdata/cius-ro/schematron/1.0.9",
		cenFiles:  []string{"EN16931-model.sch", "EN16931-syntax.sch"},
	},
	{
		authority: "NLCIUS SI-UBL 2.0.3.2 (UBL)",
		master:    "testdata/nlcius/schematron/ubl/si-ubl-2.0.3.2.sch",
		dist:      "testdata/nlcius/schematron/ubl",
		cenFiles:  []string{"EN16931-model.sch", "EN16931-syntax.sch"},
	},
	{
		authority: "NLCIUS 1.0.3 (CII)",
		master:    "testdata/nlcius/schematron/cii/nlcius-cii-1.0.3.sch",
		dist:      "testdata/nlcius/schematron/cii",
		cenFiles:  []string{"EN16931-CII-model.sch", "EN16931-CII-syntax.sch"},
	},
}

// TestCIUSCopyOmissionsAreClassifiedFromTheArtefacts re-derives the whole table:
// the release each copy was taken from, and the two lists, per CEN file.
//
// It re-derives rather than spot-checks because the alternative is a table that
// drifts from the artefacts it claims to describe, which is the failure mode PRs
// 19–28 kept finding. Both directions: an identifier the derivation puts in a list
// and the table does not fails, and so does one the table names that the derivation
// does not.
func TestCIUSCopyOmissionsAreClassifiedFromTheArtefacts(t *testing.T) {
	if _, err := os.Stat(ccCENDir); err != nil {
		t.Skip("CEN artefacts not present (make en16931-artefacts)")
	}
	if _, err := os.Stat(filepath.Join(ccCENDir, ".git", "shallow")); err == nil {
		t.Fatalf("%s is a shallow clone. This test pins each copy to the CEN release it was "+
			"taken from, which is a question about the repository's history; a shallow clone "+
			"has no releases to pin to. Run `make en16931-artefacts`", ccCENDir)
	}
	if _, err := os.Stat("testdata/cius-pt/schematron/2.1.1"); err != nil {
		t.Skip("CIUS Schematrons not present (make cius-schematron)")
	}

	byCopy := map[string]ccCopy{}
	for _, c := range ccCopies {
		byCopy[c.authority] = c
	}
	byRecord := map[string]ciusCENCopyOmission{}
	for _, o := range ciusCENCopyOmissions {
		if _, dup := byRecord[o.authority]; dup {
			t.Fatalf("ciusCENCopyOmissions names %q twice", o.authority)
		}
		byRecord[o.authority] = o
	}
	// Every authority ciusCENCopyVerdicts knows about must appear here too, said
	// either way. An authority that is simply absent is how a rule set comes to have
	// a hole nobody can see (C27, C33, C39).
	for _, v := range ciusCENCopyVerdicts {
		if _, ok := byRecord[v.authority]; !ok {
			t.Errorf("ciusCENCopyVerdicts records %q and ciusCENCopyOmissions does not mention it",
				v.authority)
		}
	}
	for _, o := range ciusCENCopyOmissions {
		if o.classified == (o.notClassified != "") {
			t.Errorf("%s: classified=%v and notClassified=%q disagree; an authority whose "+
				"omissions are not classified must say why, and one whose omissions are "+
				"classified must not", o.authority, o.classified, o.notClassified)
		}
	}

	releasesUBL := ccCENReleases(t, ccCENUBLPairs)
	releasesCII := ccCENReleases(t, ccCENCIIPairs)
	t.Logf("CEN releases: %d tagged, %s (%s) .. %s (%s)", len(releasesUBL),
		releasesUBL[0].tag, releasesUBL[0].date,
		releasesUBL[len(releasesUBL)-1].tag, releasesUBL[len(releasesUBL)-1].date)

	seen := map[string]bool{}
	for _, oc := range ccOmissionCopies {
		seen[oc.authority] = true
		want, ok := byRecord[oc.authority]
		if !ok {
			t.Errorf("%s is read here and ciusCENCopyOmissions does not record it", oc.authority)
			continue
		}
		if !want.classified {
			t.Errorf("%s is read here and the table records it as unclassified: %s",
				oc.authority, want.notClassified)
			continue
		}
		copy, ok := byCopy[oc.authority]
		if !ok {
			t.Fatalf("%s has no entry in ccCopies", oc.authority)
		}
		if len(copy.pairs) != len(oc.cenFiles) {
			t.Fatalf("%s: %d copied groups and %d CEN files named for them",
				oc.authority, len(copy.pairs), len(oc.cenFiles))
		}
		releases := releasesUBL
		if copy.syntax == "CII" {
			releases = releasesCII
		}

		cius := ccIndex(ccReadPairs(t, "", copy.pairs, copy.patterns))
		copied := map[string]bool{}
		for _, f := range oc.cenFiles {
			copied[f] = true
		}
		nowPerFile := ccCENNowPerFile(t, releases)

		gotTag, gotThrough, gotDate, carried, differing := ccPinRelease(t, oc.authority, cius, releases, copied)
		if gotTag != want.release || gotThrough != want.releaseThrough || gotDate != want.releaseDate {
			t.Errorf("%s: derived release %s..%q (%s); the table says %s..%q (%s)",
				oc.authority, gotTag, gotThrough, gotDate,
				want.release, want.releaseThrough, want.releaseDate)
		}
		if carried != want.carried || differing != want.differing {
			t.Errorf("%s: derived carried=%d differing=%d; the table says carried=%d differing=%d",
				oc.authority, carried, differing, want.carried, want.differing)
		}

		// Every CEN identifier the authority's whole distribution publishes. Read off
		// every .sch under it and not off its copy of CEN's files: AT/eSPap moved six
		// CEN rules into a pattern of its own, and a reading that looked only at the
		// copy would report all six as dropped when the rule set evaluates them.
		published := map[string]bool{}
		nowAll := map[string]ccAssert{}
		for _, ids := range nowPerFile {
			for id, a := range ids {
				nowAll[id] = a
			}
		}
		for _, id := range ccCENIdentifiersUnder(t, oc.dist, nowAll) {
			published[id] = true
		}
		included := ccMasterIncludes(t, oc.master)

		var pin map[string]map[string]ccAssert
		for _, r := range releases {
			if r.tag == want.release {
				pin = r.perFile
			}
		}
		if pin == nil {
			t.Fatalf("%s: the table names release %s and CEN has no such tag", oc.authority, want.release)
		}

		byFile := map[string]ciusCENFileOmission{}
		for _, f := range want.files {
			byFile[f.cenFile] = f
		}
		var files []string
		for f := range nowPerFile {
			files = append(files, f)
		}
		sort.Strings(files)
		for _, f := range files {
			rec, ok := byFile[f]
			if !ok {
				t.Errorf("%s: CEN publishes %s and the table says nothing about it", oc.authority, f)
				continue
			}
			delete(byFile, f)
			if rec.copied != copied[f] {
				t.Errorf("%s: %s copied=%v in the table, %v derived", oc.authority, f, rec.copied, copied[f])
			}
			wantFetched := copied[f] || !included[f]
			if rec.fetched != wantFetched {
				t.Errorf("%s: %s fetched=%v in the table; the authority's master %s include a copy "+
					"and this repository %s vendor one", oc.authority, f, rec.fetched,
					map[bool]string{true: "does", false: "does not"}[included[f]],
					map[bool]string{true: "does", false: "does not"}[copied[f]])
			}
			if !wantFetched {
				if len(rec.dropped)+len(rec.postdates) != 0 {
					t.Errorf("%s: %s is not vendored here and the table names %d identifiers for it; "+
						"a file nobody read carries no claim", oc.authority, f,
						len(rec.dropped)+len(rec.postdates))
				}
				continue
			}
			var dropped, postdates []string
			for id := range nowPerFile[f] {
				if published[id] {
					continue
				}
				if _, hadIt := pin[f][id]; hadIt {
					dropped = append(dropped, id)
				} else {
					postdates = append(postdates, id)
				}
			}
			sort.Strings(dropped)
			sort.Strings(postdates)
			ccSameSet(t, fmt.Sprintf("%s %s dropped", oc.authority, f), dropped, rec.dropped)
			ccSameSet(t, fmt.Sprintf("%s %s postdates", oc.authority, f), postdates, rec.postdates)
		}
		for f := range byFile {
			t.Errorf("%s: the table names CEN file %s and CEN publishes no such file", oc.authority, f)
		}

		nd, np := 0, 0
		for _, f := range want.files {
			nd += len(f.dropped)
			np += len(f.postdates)
		}
		t.Logf("%-38s vendored CEN %s%s (%s), %d CEN identifiers carried, %d differing: "+
			"dropped %d, postdates %d", oc.authority, want.release,
			ccThrough(want.releaseThrough), want.releaseDate, want.carried, want.differing, nd, np)
	}
	for _, o := range ciusCENCopyOmissions {
		if o.classified && !seen[o.authority] {
			t.Errorf("ciusCENCopyOmissions classifies %q and no copy is read for it", o.authority)
		}
	}
}

func ccThrough(s string) string {
	if s == "" {
		return ""
	}
	return " .. " + s
}

// ccRelease is CEN's rule set at one tagged release, per file.
type ccRelease struct {
	tag, date string
	perFile   map[string]map[string]ccAssert
}

// ccCENReleases resolves CEN's rule set at every tag, oldest first.
//
// Tags rather than commits: "which release did this authority vendor" is a question
// with a nameable answer, and the working commits between two tags are drafts nobody
// could have downloaded. The condition classification next door does walk every
// commit, because there the question is "did CEN ever write this", which a draft
// answers as well as a release.
func ccCENReleases(t *testing.T, pairs []ccPair) []ccRelease {
	t.Helper()
	type tagRev struct{ tag, date, rev string }
	var tags []tagRev
	for _, tag := range ccGitLines(t, "tag") {
		rev := ccGitLines(t, "rev-list", "-1", tag)
		if len(rev) != 1 {
			t.Fatalf("git rev-list -1 %s: %v", tag, rev)
		}
		d := ccGitLines(t, "log", "-1", "--format=%cI", rev[0])
		if len(d) != 1 {
			t.Fatalf("git log -1 %s: %v", rev[0], d)
		}
		tags = append(tags, tagRev{tag, d[0][:10], rev[0]})
	}
	if len(tags) == 0 {
		t.Fatalf("%s has no tags; the release pin has nothing to pin to", ccCENDir)
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].date < tags[j].date })

	var want []string
	for _, tr := range tags {
		for _, p := range pairs {
			want = append(want, tr.rev+":"+p.abstract)
			if p.binding != "" {
				want = append(want, tr.rev+":"+p.binding)
			}
		}
	}
	blobs := ccGitCatFile(t, want)

	out := make([]ccRelease, 0, len(tags))
	for _, tr := range tags {
		per := map[string]map[string]ccAssert{}
		for _, p := range pairs {
			ab, ok := blobs[tr.rev+":"+p.abstract]
			if !ok {
				continue
			}
			per[filepath.Base(p.abstract)] = ccIndex(ccResolve(ab, blobs[tr.rev+":"+p.binding], nil))
		}
		out = append(out, ccRelease{tag: tr.tag, date: tr.date, perFile: per})
	}
	return out
}

// ccCENNowPerFile is CEN's current rule set, per file. It is read from the working
// tree rather than from the newest tag: the working tree is what Coverage and the
// rule engine are written against, and an identifier CEN has added since its last
// tag is one this package may already report.
func ccCENNowPerFile(t *testing.T, releases []ccRelease) map[string]map[string]ccAssert {
	t.Helper()
	pairs := ccCENUBLPairs
	if _, ok := releases[0].perFile["EN16931-CII-model.sch"]; ok {
		pairs = ccCENCIIPairs
	}
	out := map[string]map[string]ccAssert{}
	for _, p := range pairs {
		out[filepath.Base(p.abstract)] = ccIndex(ccReadPairs(t, ccCENDir, []ccPair{p}, nil))
	}
	return out
}

// ccPinRelease derives which CEN release a copy was taken from, from the copy.
//
// Two facts, in this order. A release that does not publish an identifier the copy
// carries cannot be the one the copy was taken from — a hard exclusion, and what
// rules out every release older than the newest identifier in the copy. Among the
// releases that survive it, the one whose assertions the copy reproduces most
// closely, comparing the whole assertion rather than the three axes the condition
// classification uses: a reworded message is as good a version fingerprint as a
// rewritten test, and this comparison is not deciding whether a difference is
// national.
//
// It returns a run of tags rather than one when CEN republished the files unchanged,
// because saying "1.3.4 through 1.3.6" is the honest form of evidence that cannot
// tell them apart.
func ccPinRelease(t *testing.T, authority string, cius map[string]ccAssert,
	releases []ccRelease, copied map[string]bool) (first, through, date string, carried, differing int) {
	t.Helper()
	ever := map[string]bool{}
	for _, r := range releases {
		for f := range copied {
			for id := range r.perFile[f] {
				ever[id] = true
			}
		}
	}
	shared := map[string]ccAssert{}
	for id, a := range cius {
		if ever[id] {
			shared[id] = a
		}
	}

	type score struct {
		unpublished, differing int
		tag, date              string
	}
	var scored []score
	for _, r := range releases {
		idx := map[string]ccAssert{}
		for f := range copied {
			for id, a := range r.perFile[f] {
				idx[id] = a
			}
		}
		var un, di int
		for id, a := range shared {
			cn, ok := idx[id]
			if !ok {
				un++
				continue
			}
			if a.ctx != cn.ctx || a.kind != cn.kind || a.flag != cn.flag ||
				a.test != cn.test || a.msg != cn.msg {
				di++
			}
		}
		scored = append(scored, score{un, di, r.tag, r.date})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].unpublished != scored[j].unpublished {
			return scored[i].unpublished < scored[j].unpublished
		}
		if scored[i].differing != scored[j].differing {
			return scored[i].differing < scored[j].differing
		}
		return scored[i].date < scored[j].date
	})
	best := scored[0]
	if best.unpublished != 0 {
		t.Fatalf("%s: no CEN release publishes every identifier its copy carries (best is %s, "+
			"short by %d). The copy is not a copy of a CEN release, or the files it is read "+
			"from are the wrong ones", authority, best.tag, best.unpublished)
	}
	var tied []score
	for _, s := range scored {
		if s.unpublished == best.unpublished && s.differing == best.differing {
			tied = append(tied, s)
		}
	}
	sort.Slice(tied, func(i, j int) bool { return tied[i].date < tied[j].date })
	first, date = tied[0].tag, tied[0].date
	if len(tied) > 1 {
		through = tied[len(tied)-1].tag
	}
	return first, through, date, len(shared), best.differing
}

// ccMasterIncludes reads an authority's master Schematron — the file its validator is
// pointed at — and returns the basenames it <include>s.
//
// It is the authority's own manifest, and it is the only thing that distinguishes a
// CEN file the authority chose not to run from one this repository merely did not
// fetch. CIUS-RO's master and both NLCIUS masters include codelist/EN16931-*-codes.sch;
// CIUS-PT's master includes no code-list file of any name. Reading it from the
// artefact rather than keeping a list means the day an authority adds one, this
// stops claiming the file is absent.
func ccMasterIncludes(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	out := map[string]bool{}
	for _, h := range ccIncludeHrefs(t, data) {
		out[filepath.Base(h)] = true
	}
	if len(out) == 0 {
		t.Fatalf("%s has no <include>; it is not a master Schematron and the omission "+
			"classification would read every CEN file as absent", path)
	}
	return out
}

// ccIncludeHrefs reads the href of every <include> in a Schematron with an XML
// decoder. A regular expression would do here today and would stop doing it the
// first time an authority wraps an attribute across a line — the shape C31 records,
// where a guard quietly stopped guarding.
func ccIncludeHrefs(t *testing.T, data []byte) []string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = latin1Reader
	var out []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "include" {
			continue
		}
		for _, a := range se.Attr {
			if a.Name.Local == "href" {
				out = append(out, a.Value)
			}
		}
	}
}

func ccSameSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	have := map[string]bool{}
	for _, s := range want {
		have[s] = true
	}
	for _, s := range got {
		if !have[s] {
			t.Errorf("%s: derived %s and the table does not name it", what, s)
		}
		delete(have, s)
	}
	var extra []string
	for s := range have {
		extra = append(extra, s)
	}
	sort.Strings(extra)
	for _, s := range extra {
		t.Errorf("%s: the table names %s and the artefacts do not put it there", what, s)
	}
}

// TestOmittedCENIdentifiersUnderValidateCIUSPT is what the classification is worth,
// measured rather than argued.
//
// ValidateCIUSPT evaluates the whole EN 16931 rule set alongside AT/eSPap's, so it
// reports identifiers a reference CIUS-PT validator cannot: AT vendored CEN's
// validation-1.1.0 in 2018 and has not refreshed it. The two classes are not the
// same kind of finding and the counts are pinned separately for that reason:
//
//   - `postdates` is CEN's own drift. A CIUS is by construction a restriction of
//     EN 16931, so a rule CEN has added since 2018 applies to a Portuguese invoice
//     whether or not AT's copy has caught up. Reporting it is right, and the number
//     is here so that a change to it is visible rather than absorbed.
//   - `dropped` is AT's decision, and it is the class that would be suppressible if
//     AT published something covering the same ground. It does not: the code-list
//     tier has no Portuguese counterpart at all, and the BR-AE/G/IC/O/Z families
//     were deleted outright while AT's own VAT-category rules cover only S, E and
//     AA. Suppressing them would turn a divergence from AT's validator into a
//     class of invoice nothing checks, which is the worse of the two.
//
// The numbers are floors on the corpus, not equalities: a corpus that grows makes
// them grow. They are here to make a change in either direction a red build.
func TestOmittedCENIdentifiersUnderValidateCIUSPT(t *testing.T) {
	skipWithoutCorpus(t)
	dropped, postdates := map[string]bool{}, map[string]bool{}
	for _, o := range ciusCENCopyOmissions {
		if o.source != SourceCIUSPT || !o.classified {
			continue
		}
		for _, f := range o.files {
			for _, id := range f.dropped {
				dropped[id] = true
			}
			for _, id := range f.postdates {
				postdates[id] = true
			}
		}
	}
	if len(dropped) == 0 || len(postdates) == 0 {
		t.Fatalf("ciusCENCopyOmissions holds no CIUS-PT record: dropped=%d postdates=%d",
			len(dropped), len(postdates))
	}

	files, nDropped, nPostdates := 0, 0, 0
	docsDropped, docsPostdates := map[string]bool{}, map[string]bool{}
	rulesDropped, rulesPostdates := map[string]bool{}, map[string]bool{}
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".xml") {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}
		files++
		rep, verr := ValidateCIUSPT(context.Background(), data)
		if verr != nil {
			return nil
		}
		for _, v := range rep.Violations {
			if v.Source != SourceEN16931 || v.Severity != SeverityFatal {
				continue
			}
			switch {
			case dropped[v.Rule]:
				nDropped++
				docsDropped[p] = true
				rulesDropped[v.Rule] = true
			case postdates[v.Rule]:
				nPostdates++
				docsPostdates[p] = true
				rulesPostdates[v.Rule] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	atLeast(t, "omitted-identifier corpus sweep", files, minCorpusDocuments)
	atLeast(t, "findings under identifiers AT/eSPap dropped", nDropped, minCIUSPTDroppedFindings)
	atLeast(t, "findings under identifiers that postdate AT/eSPap's copy", nPostdates, minCIUSPTPostdatesFindings)
	t.Logf("ValidateCIUSPT over %d documents: %d fatal EN 16931 findings under %d identifiers "+
		"AT/eSPap dropped from CEN validation-1.1.0 (%d documents), %d under %d identifiers that "+
		"postdate it (%d documents)", files,
		nDropped, len(rulesDropped), len(docsDropped),
		nPostdates, len(rulesPostdates), len(docsPostdates))
}

// TestATInstancesFailOnlyIdentifiersATDoesNotPublish is the sharpest evidence in this
// file, and it is a statement about documents rather than about artefacts.
//
// AT/eSPap publishes 20 sample instances as conformant. ValidateCIUSPT reports fatal
// EN 16931 findings on all of them — and every single one of those findings is under
// an identifier AT's own rule set does not contain. Not one is a rule AT publishes
// and this package got wrong.
//
// That is the whole shape of the divergence in one measurement. It also says what the
// divergence is *not*: if any finding here were under an identifier AT does publish,
// this package would be over-reporting a Portuguese rule, which is the C29/C32/C42
// defect and would have to be fixed rather than recorded.
func TestATInstancesFailOnlyIdentifiersATDoesNotPublish(t *testing.T) {
	files, _ := filepath.Glob("testdata/cius-pt/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("CIUS-PT corpus not present (make cius-oracles)")
	}
	atLeast(t, "CIUS-PT corpus", len(files), minCIUSPTInstances)

	omitted := map[string]string{}
	for _, o := range ciusCENCopyOmissions {
		if o.source != SourceCIUSPT || !o.classified {
			continue
		}
		for _, f := range o.files {
			for _, id := range f.dropped {
				omitted[id] = "dropped"
			}
			for _, id := range f.postdates {
				omitted[id] = "postdates"
			}
		}
	}
	byRule := map[string]int{}
	accused, findings := 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rep, verr := ValidateCIUSPT(context.Background(), data)
		if verr != nil {
			t.Fatalf("%s: %v", f, verr)
		}
		hit := false
		for _, v := range rep.Violations {
			if v.Source != SourceEN16931 || v.Severity != SeverityFatal {
				continue
			}
			findings++
			hit = true
			why, ok := omitted[v.Rule]
			if !ok {
				t.Errorf("%s: fatal %s on an instance AT/eSPap publishes as conformant, and AT's "+
					"rule set does publish %s. That is this package over-reporting a rule the "+
					"authority has, not a rule the authority lacks", filepath.Base(f), v.Rule, v.Rule)
				continue
			}
			byRule[v.Rule+" ("+why+")"]++
		}
		if hit {
			accused++
		}
	}
	var keys []string
	for k := range byRule {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s x%d; ", k, byRule[k])
	}
	t.Logf("AT/eSPap's %d published conformant instances: %d carry %d fatal EN 16931 findings, "+
		"all under identifiers AT does not publish — %s", len(files), accused, findings,
		strings.TrimSuffix(b.String(), "; "))
}

// TestNoDroppedCENIdentifierIsSuppressed is the false-negative guard, and it asserts
// the decision this PR took rather than a property of the artefacts.
//
// Of the 114 CEN identifiers AT/eSPap dropped, none is suppressed under
// ValidateCIUSPT, because none has a Portuguese replacement covering the same ground.
// The DT-CIUS-PT-157..177 tier does replace CEN's arithmetic rules, but with a
// ±1.00 € tolerance: measured across this corpus it fails to fire on 30 documents
// where the CEN rule it displaced does, so honouring it instead would leave those
// documents reported by nothing. Suppressing a rule whose replacement is weaker
// converts a divergence from AT's validator into a false negative, and a false
// negative is worse because nothing reports it.
//
// If that decision is ever revisited, this test is what has to change, and it names
// what has to be true first.
func TestNoDroppedCENIdentifierIsSuppressed(t *testing.T) {
	dropped := map[string]bool{}
	for _, o := range ciusCENCopyOmissions {
		if o.source != SourceCIUSPT || !o.classified {
			continue
		}
		for _, f := range o.files {
			for _, id := range f.dropped {
				dropped[id] = true
			}
		}
	}
	// A document that trips several of the dropped identifiers at once: a bogus VAT
	// category code, a bogus country code and totals that do not add up.
	doc := ptOmissionFixture
	rep, err := ValidateCIUSPT(context.Background(), []byte(doc))
	if err != nil {
		t.Fatalf("%v", err)
	}
	core, cerr := Validate(context.Background(), []byte(doc), ProfileEN16931)
	if cerr != nil {
		t.Fatalf("%v", cerr)
	}
	got := map[string]bool{}
	for _, v := range rep.Violations {
		if v.Source == SourceEN16931 {
			got[v.Rule] = true
		}
	}
	var missing []string
	for _, v := range core.Violations {
		if v.Source != SourceEN16931 || !dropped[v.Rule] {
			continue
		}
		if !got[v.Rule] {
			missing = append(missing, v.Rule)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Errorf("Validate reports %v and ValidateCIUSPT does not. These are identifiers "+
			"AT/eSPap dropped from its copy of CEN's Schematron, and this package reports them "+
			"on purpose: no Portuguese rule covers the same ground", missing)
	}
	var fired []string
	for _, v := range core.Violations {
		if v.Source == SourceEN16931 && dropped[v.Rule] {
			fired = append(fired, v.Rule)
		}
	}
	sort.Strings(fired)
	if len(fired) < 3 {
		t.Fatalf("the fixture trips %v; it is meant to trip several identifiers AT dropped, "+
			"and a fixture that stops tripping them makes this guard vacuous", fired)
	}
	t.Logf("dropped identifiers still reported under ValidateCIUSPT: %v", fired)
}

// ptReplacement is one CEN rule AT/eSPap dropped and the Portuguese rule that asks
// the same question. The pairs are read off the two artefacts' own message texts,
// which name the business terms: DT-CIUS-PT-163 says "Invoice total amount without
// VAT (BT-109) = Σ Invoice line net amount (BT-131) - Sum of allowances on document
// level (BT-107) + Sum of charges on document level (BT-108)", and so does BR-CO-13.
//
// This is the whole set. The other 103 identifiers AT dropped have no Portuguese
// counterpart of any kind: the 19 `BR-CL-*` because AT's rule set runs no code-list
// file, and the `BR-AE/G/IC/O/Z-*` families because AT deleted them outright while
// its own VAT-category rules cover only S, E and AA.
type ptReplacement struct{ cen, pt string }

var ptReplacements = []ptReplacement{
	{"BR-CO-10", "DT-CIUS-PT-160"}, // Σ line net amounts
	{"BR-CO-11", "DT-CIUS-PT-161"}, // Σ document-level allowances
	{"BR-CO-12", "DT-CIUS-PT-162"}, // Σ document-level charges
	{"BR-CO-13", "DT-CIUS-PT-163"}, // total without VAT
	{"BR-CO-14", "DT-CIUS-PT-164"}, // total VAT amount
	{"BR-CO-15", "DT-CIUS-PT-165"}, // total with VAT
	{"BR-CO-16", "DT-CIUS-PT-166"}, // amount due for payment
	{"BR-CO-17", "DT-CIUS-PT-167"}, // VAT category tax amount
	{"BR-S-08", "DT-CIUS-PT-173"},  // standard-rated taxable amount
	{"BR-S-09", "DT-CIUS-PT-174"},  // standard-rated tax amount
	{"BR-E-08", "DT-CIUS-PT-175"},  // exempt taxable amount
}

// TestATsArithmeticReplacementsAreWeakerThanCENs is the measurement that decides
// whether the eleven dropped rules above may be suppressed. They may not.
//
// AT/eSPap's replacements carry a ±1.00 € acceptance range where CEN's are exact
// identities. So they are not the same rule under a new name: they are a relaxation,
// and an invoice whose totals are wrong by less than a euro passes AT's and fails
// CEN's. This sweeps the corpus and counts the documents where that difference is
// live — where this package reports the CEN rule and AT's replacement stays silent.
//
// Every one of those documents is one that would stop being reported by anything if
// the suppression were made. That is the false negative the audit warns against, and
// counting it is what turns "AT put something in its place" from a plausible
// argument into a checked one.
//
// The assertion is an inequality on purpose: the number must stay above zero, because
// a zero would mean the corpus no longer witnesses the difference and this test had
// quietly stopped being evidence for anything.
func TestATsArithmeticReplacementsAreWeakerThanCENs(t *testing.T) {
	skipWithoutCorpus(t)
	files, cenOnly, both := 0, 0, 0
	perRule := map[string]int{}
	docs := map[string]bool{}
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".xml") {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("%s: %v", p, rerr)
		}
		files++
		// UBL only. AT/eSPap publishes no CII binding, so its replacements do not run
		// on a Factur-X invoice at all and counting one would measure C36's defect
		// rather than the tolerance.
		parsed, perr := parseEN16931(newRun(context.Background()), data)
		if perr != nil || parsed.inv == nil || parsed.inv.syntax != "UBL" {
			return nil
		}
		rep, verr := ValidateCIUSPT(context.Background(), data)
		if verr != nil {
			return nil
		}
		have := map[string]bool{}
		for _, v := range rep.Violations {
			have[v.Rule] = true
		}
		for _, r := range ptReplacements {
			if !have[r.cen] {
				continue
			}
			if have[r.pt] {
				both++
				continue
			}
			cenOnly++
			perRule[r.cen]++
			docs[p] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	atLeast(t, "arithmetic-replacement corpus sweep", files, minCorpusDocuments)
	if cenOnly == 0 {
		t.Errorf("no UBL document in the corpus reports a CEN arithmetic rule that AT/eSPap's "+
			"replacement does not. Either the corpus lost them or the replacements stopped being "+
			"weaker, and until one of those is established this test is no longer evidence that "+
			"suppressing %d identifiers would create a false negative", len(ptReplacements))
	}
	var keys []string
	for k := range perRule {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s x%d; ", k, perRule[k])
	}
	t.Logf("AT/eSPap's ±1.00 EUR replacements: %d (rule, document) pairs where CEN's rule fires "+
		"and AT's does not, across %d documents; %d where both fire — %s",
		cenOnly, len(docs), both, strings.TrimSuffix(b.String(), "; "))
}

// ptOmissionFixture is a UBL invoice that is CIUS-PT-shaped and trips several of the
// CEN identifiers AT/eSPap dropped: BR-CL-14/15 (a country code that is not ISO
// 3166-1), BR-CL-17/18 (a VAT category code outside EN 16931's restricted BT-118
// list) and BR-CO-15 (totals that do not add up). AT's own rule set contains no rule
// that reports any of them.
const ptOmissionFixture = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:2.1.1</cbc:CustomizationID>
  <cbc:ID>FT 2024/1</cbc:ID>
  <cbc:IssueDate>2024-01-15</cbc:IssueDate>
  <cbc:DueDate>2024-02-15</cbc:DueDate>
  <cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>
  <cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
  <cbc:BuyerReference>PT-BUYER-1</cbc:BuyerReference>
  <cac:AccountingSupplierParty><cac:Party>
    <cbc:EndpointID schemeID="9946">PT500000000</cbc:EndpointID>
    <cac:PartyName><cbc:Name>Vendedor Lda</cbc:Name></cac:PartyName>
    <cac:PostalAddress><cbc:StreetName>Rua 1</cbc:StreetName><cbc:CityName>Lisboa</cbc:CityName>
      <cbc:PostalZone>1000-001</cbc:PostalZone>
      <cac:Country><cbc:IdentificationCode>XX</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
    <cac:PartyTaxScheme><cbc:CompanyID>PT500000000</cbc:CompanyID>
      <cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
    <cac:PartyLegalEntity><cbc:RegistrationName>Vendedor Lda</cbc:RegistrationName>
      <cbc:CompanyID>500000000</cbc:CompanyID></cac:PartyLegalEntity>
    <cac:Contact><cbc:Name>Ana</cbc:Name><cbc:Telephone>212345678</cbc:Telephone>
      <cbc:ElectronicMail>ana@example.pt</cbc:ElectronicMail></cac:Contact>
  </cac:Party></cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty><cac:Party>
    <cbc:EndpointID schemeID="9946">PT500000001</cbc:EndpointID>
    <cac:PartyIdentification><cbc:ID>PT500000001</cbc:ID></cac:PartyIdentification>
    <cac:PartyName><cbc:Name>Comprador SA</cbc:Name></cac:PartyName>
    <cac:PostalAddress><cbc:StreetName>Rua 2</cbc:StreetName><cbc:CityName>Porto</cbc:CityName>
      <cbc:PostalZone>4000-001</cbc:PostalZone>
      <cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
    <cac:PartyTaxScheme><cbc:CompanyID>PT500000001</cbc:CompanyID>
      <cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
    <cac:PartyLegalEntity><cbc:RegistrationName>Comprador SA</cbc:RegistrationName></cac:PartyLegalEntity>
    <cac:Contact><cbc:Name>Rui</cbc:Name><cbc:Telephone>223456789</cbc:Telephone>
      <cbc:ElectronicMail>rui@example.pt</cbc:ElectronicMail></cac:Contact>
  </cac:Party></cac:AccountingCustomerParty>
  <cac:Delivery><cbc:ActualDeliveryDate>2024-01-15</cbc:ActualDeliveryDate>
    <cac:DeliveryLocation><cac:Address><cbc:CityName>Porto</cbc:CityName>
      <cac:Country><cbc:IdentificationCode>PT</cbc:IdentificationCode></cac:Country></cac:Address>
    </cac:DeliveryLocation></cac:Delivery>
  <cac:PaymentMeans><cbc:PaymentMeansCode>30</cbc:PaymentMeansCode>
    <cac:PayeeFinancialAccount><cbc:ID>PT50000201231234567890154</cbc:ID></cac:PayeeFinancialAccount>
  </cac:PaymentMeans>
  <cac:TaxTotal><cbc:TaxAmount currencyID="EUR">23.00</cbc:TaxAmount>
    <cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount>
      <cbc:TaxAmount currencyID="EUR">23.00</cbc:TaxAmount>
      <cac:TaxCategory><cbc:ID>QQ</cbc:ID><cbc:Percent>23</cbc:Percent>
        <cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>
  </cac:TaxTotal>
  <cac:LegalMonetaryTotal>
    <cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
    <cbc:TaxExclusiveAmount currencyID="EUR">100.00</cbc:TaxExclusiveAmount>
    <cbc:TaxInclusiveAmount currencyID="EUR">999.00</cbc:TaxInclusiveAmount>
    <cbc:PayableAmount currencyID="EUR">999.00</cbc:PayableAmount>
  </cac:LegalMonetaryTotal>
  <cac:InvoiceLine><cbc:ID>1</cbc:ID>
    <cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity>
    <cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
    <cac:Item><cbc:Name>Servico</cbc:Name>
      <cac:ClassifiedTaxCategory><cbc:ID>QQ</cbc:ID><cbc:Percent>23</cbc:Percent>
        <cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:ClassifiedTaxCategory></cac:Item>
    <cac:Price><cbc:PriceAmount currencyID="EUR">100.00</cbc:PriceAmount></cac:Price>
  </cac:InvoiceLine>
</Invoice>`
