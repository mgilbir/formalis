package formalis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFatturaPACorpus is the FP=0 oracle: every official FatturaPA sample
// (phax/phive-rules, all "good" cases) must validate with no violations. Skips
// when the corpus is absent (run `make cius-oracles`).
func TestFatturaPACorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/fatturapa/testsuite/*.xml")
	if len(files) == 0 {
		t.Skip("FatturaPA corpus not present (make cius-oracles)")
	}
	atLeast(t, "FatturaPA corpus", len(files), minFatturaPAInstances)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := IsFatturaPA(data)
		if err != nil {
			t.Errorf("%s: could not be read: %v", filepath.Base(f), err)
			continue
		}
		if !ok {
			t.Errorf("%s: not recognised as a FatturaPA document", filepath.Base(f))
			continue
		}
		if v := ValidateFatturaPA(context.Background(), data).Violations; len(v) != 0 {
			t.Errorf("%s: expected 0 FatturaPA violations on a conformant sample, got %d (first %s: %s)",
				filepath.Base(f), len(v), v[0].Rule, v[0].Message)
		}
	}
}

// minimalFatturaPA is a small conformant FatturaElettronica (FPR12) with the
// mandatory structure, for mutation testing.
const minimalFatturaPA = `<p:FatturaElettronica versione="FPR12" xmlns:p="http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2">
<FatturaElettronicaHeader>
 <DatiTrasmissione><IdTrasmittente><IdPaese>IT</IdPaese><IdCodice>01234567890</IdCodice></IdTrasmittente><ProgressivoInvio>00001</ProgressivoInvio><FormatoTrasmissione>FPR12</FormatoTrasmissione><CodiceDestinatario>0000000</CodiceDestinatario></DatiTrasmissione>
 <CedentePrestatore>
  <DatiAnagrafici><IdFiscaleIVA><IdPaese>IT</IdPaese><IdCodice>01234567890</IdCodice></IdFiscaleIVA><Anagrafica><Denominazione>Venditore SpA</Denominazione></Anagrafica><RegimeFiscale>RF01</RegimeFiscale></DatiAnagrafici>
  <Sede><Indirizzo>Via Roma 1</Indirizzo><CAP>00100</CAP><Comune>Roma</Comune><Provincia>RM</Provincia><Nazione>IT</Nazione></Sede>
 </CedentePrestatore>
 <CessionarioCommittente>
  <DatiAnagrafici><CodiceFiscale>09876543210</CodiceFiscale><Anagrafica><Denominazione>Cliente Srl</Denominazione></Anagrafica></DatiAnagrafici>
  <Sede><Indirizzo>Via Milano 2</Indirizzo><CAP>20100</CAP><Comune>Milano</Comune><Provincia>MI</Provincia><Nazione>IT</Nazione></Sede>
 </CessionarioCommittente>
</FatturaElettronicaHeader>
<FatturaElettronicaBody>
 <DatiGenerali><DatiGeneraliDocumento><TipoDocumento>TD01</TipoDocumento><Divisa>EUR</Divisa><Data>2024-01-15</Data><Numero>INV-1</Numero><ImportoTotaleDocumento>122.00</ImportoTotaleDocumento></DatiGeneraliDocumento></DatiGenerali>
 <DatiBeniServizi>
  <DettaglioLinee><NumeroLinea>1</NumeroLinea><Descrizione>Prodotto</Descrizione><PrezzoUnitario>100.00</PrezzoUnitario><PrezzoTotale>100.00</PrezzoTotale><AliquotaIVA>22.00</AliquotaIVA></DettaglioLinee>
  <DatiRiepilogo><AliquotaIVA>22.00</AliquotaIVA><ImponibileImporto>100.00</ImponibileImporto><Imposta>22.00</Imposta></DatiRiepilogo>
 </DatiBeniServizi>
</FatturaElettronicaBody>
</p:FatturaElettronica>`

func TestFatturaPAMutations(t *testing.T) {
	if v := ValidateFatturaPA(context.Background(), []byte(minimalFatturaPA)).Violations; len(v) != 0 {
		t.Fatalf("baseline FatturaPA not clean: %d (first %s: %s)", len(v), v[0].Rule, v[0].Message)
	}
	cases := []struct{ name, from, to, want string }{
		{"bad format", "<FormatoTrasmissione>FPR12</FormatoTrasmissione>", "<FormatoTrasmissione>XXX12</FormatoTrasmissione>", "FPA-format"},
		{"no transmitter", "<IdTrasmittente><IdPaese>IT</IdPaese><IdCodice>01234567890</IdCodice></IdTrasmittente>", "<IdTrasmittente></IdTrasmittente>", "FPA-transmitter"},
		{"no destination", "<CodiceDestinatario>0000000</CodiceDestinatario>", "<CodiceDestinatario></CodiceDestinatario>", "FPA-destination"},
		{"no seller id", "<IdFiscaleIVA><IdPaese>IT</IdPaese><IdCodice>01234567890</IdCodice></IdFiscaleIVA>", "", "FPA-seller-id"},
		{"bad seller regime", "<RegimeFiscale>RF01</RegimeFiscale>", "<RegimeFiscale>RF99</RegimeFiscale>", "FPA-seller-regime"},
		{"no seller address", "<Indirizzo>Via Roma 1</Indirizzo>", "", "FPA-seller-address"},
		{"no buyer id", "<CodiceFiscale>09876543210</CodiceFiscale>", "", "FPA-buyer-id"},
		{"bad doc type", "<TipoDocumento>TD01</TipoDocumento>", "<TipoDocumento>TD99</TipoDocumento>", "FPA-doctype"},
		{"bad currency", "<Divisa>EUR</Divisa>", "<Divisa>EURO</Divisa>", "FPA-currency"},
		{"no lines", "<DettaglioLinee><NumeroLinea>1</NumeroLinea><Descrizione>Prodotto</Descrizione><PrezzoUnitario>100.00</PrezzoUnitario><PrezzoTotale>100.00</PrezzoTotale><AliquotaIVA>22.00</AliquotaIVA></DettaglioLinee>", "", "FPA-lines"},
		{"no summary", "<DatiRiepilogo><AliquotaIVA>22.00</AliquotaIVA><ImponibileImporto>100.00</ImponibileImporto><Imposta>22.00</Imposta></DatiRiepilogo>", "", "FPA-summary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(minimalFatturaPA, tc.from, tc.to, 1)
			if broken == minimalFatturaPA {
				t.Fatalf("mutation string not found: %q", tc.from)
			}
			if !hasFacturXRule(ValidateFatturaPA(context.Background(), []byte(broken)).Violations, tc.want) {
				t.Errorf("expected %s to fire; got %v", tc.want, ValidateFatturaPA(context.Background(), []byte(broken)).Violations)
			}
		})
	}
}
