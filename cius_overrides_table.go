package formalis

// Code generated from the CIUS Schematrons and CEN/TC 434's own git history by
// testdata/cius-condition-overrides/gen.py; DO NOT EDIT. Regenerate with
// `make cius-condition-overrides`.
//
// Five of the CIUS this package validates against ship a *copy* of CEN's Schematron
// rather than referencing it, and a copy can be edited. This file is the answer to
// "which CEN conditions did each authority write itself", derived by resolving each
// copy and asking CEN's repository whether it ever published what the copy carries.
// A condition CEN published at some release is a stale vendored copy and not a
// national reading; one CEN never published is the authority's own, and only those
// are overridden. The reasoning is in cius_overrides.go and the derivation in the
// generator's docstring.

// ciusCENCopyVerdicts records, per authority, what its copy of CEN's Schematron
// turned out to be. Derived by testdata/cius-condition-overrides/gen.py from the
// copy and from CEN's own git history, and re-derived by
// TestCIUSCopiesOfCENAreClassifiedFromTheArtefacts on every run.
//
// stale is the count that matters for reading this table: a condition the copy
// carries, CEN's current file does not, and some CEN commit did. It is a measure of
// how old the vendored copy is and not of anything national. own is the count this
// package acts on.
var ciusCENCopyVerdicts = []ciusCENCopyVerdict{
	{
		authority: "CIUS-PT 2.1.1 (UBL)",
		source:    SourceCIUSPT,
		ships:     true,
		shared:    771,
		same:      27,
		stale:     735,
		applied:   true,
		own: map[string]string{
			"BR-23":   "polarity, test",
			"BR-E-02": "test",
			"BR-E-03": "test",
			"BR-E-04": "test",
			"BR-E-10": "context",
			"BR-S-02": "test",
			"BR-S-03": "test",
			"BR-S-04": "test",
			"BR-S-10": "context",
		},
	},
	{
		authority: "CIUS-RO 1.0.9 (UBL)",
		source:    SourceCIUSRO,
		ships:     true,
		shared:    930,
		same:      904,
		stale:     26,
		applied:   true,
		own:       map[string]string{},
	},
	{
		authority: "NLCIUS SI-UBL 2.0.3.2 (UBL)",
		source:    SourceNLCIUS,
		ships:     true,
		shared:    929,
		same:      866,
		stale:     63,
		applied:   true,
		own:       map[string]string{},
	},
	{
		authority: "NLCIUS SI-UBL G-account 1.0.2 (UBL)",
		source:    SourceNLCIUS,
		ships:     true,
		shared:    745,
		same:      700,
		stale:     45,
		applied:   true,
		own:       map[string]string{},
	},
	{
		authority: "NLCIUS 1.0.3 (CII)",
		source:    SourceNLCIUS,
		ships:     true,
		shared:    733,
		same:      628,
		stale:     105,
		applied:   true,
		own:       map[string]string{},
	},
	{
		authority:  "UBL.BE v1.31 (UBL)",
		source:     SourceUBLBE,
		ships:      true,
		shared:     250,
		same:       205,
		stale:      38,
		applied:    false,
		notApplied: "the five BR-*-08 conditions sum xs:decimal amounts across a node population reached through the parent axis, and the two BR-CL-* ones sit in <rule> elements whose contexts carry two predicates on one step, which neither this generator's context grammar nor ptDTCtxStep describes. Applying them would mean extending both and re-measuring an arithmetic comparison over the whole corpus; reported here rather than applied half-checked",
		own: map[string]string{
			"BR-AE-08": "test",
			"BR-CL-11": "test",
			"BR-CL-15": "test",
			"BR-G-08":  "test",
			"BR-IC-08": "test",
			"BR-O-08":  "test",
			"BR-Z-08":  "test",
		},
	},
	{
		authority: "SRBDT 1.0.0 (UBL)",
		source:    SourceSRBDT,
		ships:     false,
		shared:    0,
		same:      0,
		stale:     0,
		applied:   true,
		own:       map[string]string{},
	},
}

// ptOverrideModelPattern is CIUS-PT 2.1.1 (UBL)'s copy of CEN's pattern, carrying the 9
// assertions whose condition CIUS-PT 2.1.1 (UBL) wrote itself. Every <rule> of the pattern is here
// in the authority's order, because under ISO Schematron a node goes to the first
// rule whose context matches it and a rule dropped for carrying nothing would hand
// its nodes to a rule below it. The rules that carry no assertion are here for
// their contexts alone.
var ptOverrideModelPattern = ptDTPattern{
	name: "model",
	rules: []ptDTRuleSrc{
		{
			context: "cac:AdditionalDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AdditionalDocumentReference", ""}}}},
		},
		{
			context: "cac:AccountingCustomerParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingCustomerParty", ""}}}},
		},
		{
			context: "cac:AccountingCustomerParty/cac:Party/cbc:EndpointID",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"EndpointID", ""}}}},
		},
		{
			context: "cac:AccountingCustomerParty/cac:Party/cac:PostalAddress",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingCustomerParty", ""}, {"Party", ""}, {"PostalAddress", ""}}}},
		},
		{
			context: "cac:ContractDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"ContractDocumentReference", ""}}}},
		},
		{
			context: "cac:Delivery",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"Delivery", ""}}}},
		},
		{
			context: "cac:Delivery/cac:DeliveryLocation/cac:Address",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"Delivery", ""}, {"DeliveryLocation", ""}, {"Address", ""}}}},
		},
		{
			context: "cac:DespatchDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"DespatchDocumentReference", ""}}}},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'false'] | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}},
		},
		{
			context: "//ubl:Invoice/cac:AllowanceCharge[cbc:ChargeIndicator = 'true'] | //cn:CreditNote/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}, {"CreditNote", []ptDTCtxStep{{"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}},
		},
		{
			context: "cac:LegalMonetaryTotal",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"LegalMonetaryTotal", ""}}}},
		},
		{
			context: "//ubl:Invoice | //cn:CreditNote",
			paths:   []ptDTCtxPath{{"Invoice", []ptDTCtxStep{}}, {"CreditNote", []ptDTCtxStep{}}},
			asserts: []ptDTAssertSrc{
				{"BR-S-02", "assert", "(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]))", "An Invoice that contains an Invoice line (BG-25) where the Invoiced item VAT category code (BT-151) is “Standard rated” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-S-03", "assert", "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]))", "An Invoice that contains a Document level allowance (BG-20) where the Document level allowance VAT category code (BT-95) is “Standard rated” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-S-04", "assert", "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]))", "An Invoice that contains a Document level charge (BG-21) where the Document level charge VAT category code (BT-102) is “Standard rated” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-E-02", "assert", "(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:ClassifiedTaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]))", "An Invoice that contains an Invoice line (BG-25) where the Invoiced item VAT category code (BT-151) is “Exempt from VAT” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-E-03", "assert", "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='false']/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]))", "An Invoice that contains a Document level allowance (BG-20) where the Document level allowance VAT category code (BT-95) is “Exempt from VAT” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
				{"BR-E-04", "assert", "(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]) and (exists(//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID) or exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID))) or not(exists(//cac:AllowanceCharge[cbc:ChargeIndicator='true']/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]))", "An Invoice that contains a Document level charge (BG-21) where the Document level charge VAT category code (BT-102) is “Exempt from VAT” shall contain the Seller VAT Identifier (BT-31), the Seller tax registration identifier (BT-32) and/or the Seller tax representative VAT identifier (BT-63)."},
			},
		},
		{
			context: "cac:InvoiceLine | cac:CreditNoteLine",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}}}},
			asserts: []ptDTAssertSrc{
				{"BR-23", "report", "(exists(cbc:InvoicedQuantity) and not(cbc:InvoicedQuantity/@unitCode)) or (exists(cbc:CreditedQuantity) and not(cbc:CreditedQuantity/@unitCode))", "An Invoice line (BG-25) shall have an Invoiced quantity unit of measure code (BT-130)."},
			},
		},
		{
			context: "//cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'false'] | //cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'false']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'false'"}}}},
		},
		{
			context: "//cac:InvoiceLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'true'] | //cac:CreditNoteLine/cac:AllowanceCharge[cbc:ChargeIndicator = 'true']",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"AllowanceCharge", "cbc:ChargeIndicator = 'true'"}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item | cac:CreditNoteLine/cac:Item",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}}}},
		},
		{
			context: "cac:InvoiceLine/cac:InvoicePeriod | cac:CreditNoteLine/cac:InvoicePeriod",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"InvoicePeriod", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"InvoicePeriod", ""}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Price | cac:CreditNoteLine/cac:Price",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Price", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Price", ""}}}},
		},
		{
			context: "cac:InvoicePeriod",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoicePeriod", ""}}}},
		},
		{
			context: "//cac:AdditionalItemProperty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AdditionalItemProperty", ""}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item/cac:CommodityClassification/cbc:ItemClassificationCode | cac:CreditNoteLine/cac:Item/cac:CommodityClassification/cbc:ItemClassificationCode",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"CommodityClassification", ""}, {"ItemClassificationCode", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"CommodityClassification", ""}, {"ItemClassificationCode", ""}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item/cac:StandardItemIdentification/cbc:ID | cac:CreditNoteLine/cac:Item/cac:StandardItemIdentification/cbc:ID",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", ""}, {"StandardItemIdentification", ""}, {"ID", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", ""}, {"StandardItemIdentification", ""}, {"ID", ""}}}},
		},
		{
			context: "cac:OrderReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"OrderReference", ""}}}},
		},
		{
			context: "cac:OriginatorDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"OriginatorDocumentReference", ""}}}},
		},
		{
			context: "cac:PayeeParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PayeeParty", ""}}}},
		},
		{
			context: "cac:PaymentMeans[cbc:PaymentMeansCode='30' or cbc:PaymentMeansCode='58']/cac:PayeeFinancialAccount",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PaymentMeans", "cbc:PaymentMeansCode='30' or cbc:PaymentMeansCode='58'"}, {"PayeeFinancialAccount", ""}}}},
		},
		{
			context: "cac:PaymentMeans",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PaymentMeans", ""}}}},
		},
		{
			context: "cac:PaymentTerms",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"PaymentTerms", ""}}}},
		},
		{
			context: "cac:BillingReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"BillingReference", ""}}}},
		},
		{
			context: "cac:ProjectReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"ProjectReference", ""}}}},
		},
		{
			context: "cac:ReceiptDocumentReference",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"ReceiptDocumentReference", ""}}}},
		},
		{
			context: "cac:AccountingSupplierParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingSupplierParty", ""}}}},
		},
		{
			context: "cac:AccountingSupplierParty/cac:Party/cbc:EndpointID",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"EndpointID", ""}}}},
		},
		{
			context: "cac:AccountingSupplierParty/cac:Party/cac:PostalAddress",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"AccountingSupplierParty", ""}, {"Party", ""}, {"PostalAddress", ""}}}},
		},
		{
			context: "cac:TaxRepresentativeParty",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxRepresentativeParty", ""}}}},
		},
		{
			context: "cac:TaxRepresentativeParty/cac:PostalAddress",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxRepresentativeParty", ""}, {"PostalAddress", ""}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')]",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "(normalize-space(cbc:ID) = 'AA' or normalize-space(cbc:ID) = 'RED' or normalize-space(cbc:ID) = 'INT')"}}}},
		},
		{
			context: "cac:InvoiceLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'AA']/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'AA']/cac:AdditionalItemProperty/cbc:Name",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", "cac:ClassifiedTaxCategory/cbc:ID = 'AA'"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", "cac:ClassifiedTaxCategory/cbc:ID = 'AA'"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')]",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "(normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR')"}}}},
			asserts: []ptDTAssertSrc{
				{"BR-S-10", "assert", "not(cbc:TaxExemptionReason) and not(cbc:TaxExemptionReasonCode)", "A VATBReakdown (BG-23) with VAT Category code (BT-118) \"Standard rate\" shall not have a VAT exemption reason code (BT-121) or VAT exemption reason text (BT-120)."},
			},
		},
		{
			context: "cac:InvoiceLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'S' or cac:ClassifiedTaxCategory/cbc:ID = 'NOR']/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'S' or normalize-space(cbc:ID) = 'NOR']/cac:AdditionalItemProperty/cbc:Name",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", "cac:ClassifiedTaxCategory/cbc:ID = 'S' or cac:ClassifiedTaxCategory/cbc:ID = 'NOR'"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", "cac:ClassifiedTaxCategory/cbc:ID = 'S' or normalize-space(cbc:ID) = 'NOR'"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}},
		},
		{
			context: "cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')]",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"TaxTotal", ""}, {"TaxSubtotal", ""}, {"TaxCategory", "(normalize-space(cbc:ID) = 'E' or normalize-space(cbc:ID) = 'ISE')"}}}},
			asserts: []ptDTAssertSrc{
				{"BR-E-10", "assert", "exists(cbc:TaxExemptionReason) or exists(cbc:TaxExemptionReasonCode)", "A VATBReakdown (BG-23) with VAT Category code (BT-118) \"Exempt from VAT\" shall have a VAT exemption reason code (BT-121) or a VAT exemption reason text (BT-120)."},
			},
		},
		{
			context: "cac:InvoiceLine/cac:Item[(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')]/cac:AdditionalItemProperty/cbc:Name | cac:CreditNoteLine/cac:Item[(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')]/cac:AdditionalItemProperty/cbc:Name",
			paths:   []ptDTCtxPath{{"", []ptDTCtxStep{{"InvoiceLine", ""}, {"Item", "(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}, {"", []ptDTCtxStep{{"CreditNoteLine", ""}, {"Item", "(cac:ClassifiedTaxCategory/cbc:ID = 'E' or cac:ClassifiedTaxCategory/cbc:ID = 'ISE')"}, {"AdditionalItemProperty", ""}, {"Name", ""}}}},
		},
	},
}

var ptConditionOverrides = &ciusOverrides{
	authority: SourceCIUSPT,
	syntax:    "UBL",
	rules: map[string]Severity{
		"BR-23":   SeverityFatal,
		"BR-E-02": SeverityFatal,
		"BR-E-03": SeverityFatal,
		"BR-E-04": SeverityFatal,
		"BR-E-10": SeverityFatal,
		"BR-S-02": SeverityFatal,
		"BR-S-03": SeverityFatal,
		"BR-S-04": SeverityFatal,
		"BR-S-10": SeverityFatal,
	},
	patterns: []ptDTPattern{ptOverrideModelPattern},
}
