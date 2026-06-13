package utils

import "testing"

// Real Nobu static QRIS sample provided for this feature: NMID prefix "ID",
// suffix "978", terminal "A01", city MALANG.
const sampleNobuStaticQR = "00020101021126670016COM.NOBUBANK.WWW01189360050300000718990214123678721403260303UMI51440014ID.CO.QRIS.WWW0215ID20210652249780303UMI5204481253033605802ID5921TY088903 JB INDONESIA6006MALANG61056511162070703A0163041536"

func TestParseQRIS_NobuStaticSample(t *testing.T) {
	info, err := ParseQRIS(sampleNobuStaticQR)
	if err != nil {
		t.Fatalf("ParseQRIS returned error: %v", err)
	}

	if info.NMID != "ID2021065224978" {
		t.Errorf("NMID = %q, want %q", info.NMID, "ID2021065224978")
	}
	if info.TerminalID != "A01" {
		t.Errorf("TerminalID = %q, want %q", info.TerminalID, "A01")
	}
	if info.MerchantName != "TY088903 JB INDONESIA" {
		t.Errorf("MerchantName = %q, want %q", info.MerchantName, "TY088903 JB INDONESIA")
	}
	if info.MerchantCity != "MALANG" {
		t.Errorf("MerchantCity = %q, want %q", info.MerchantCity, "MALANG")
	}
	if info.MerchantCategoryCode != "4812" {
		t.Errorf("MerchantCategoryCode = %q, want %q", info.MerchantCategoryCode, "4812")
	}
	if !info.IsStatic {
		t.Errorf("IsStatic = false, want true (tag 01 == 11)")
	}
}

func TestParseQRIS_Errors(t *testing.T) {
	cases := []struct {
		name string
		qr   string
	}{
		{"empty", ""},
		{"truncated header", "0002"},
		{"length overruns", "0050AB"},
		{"no nmid", "00020101021152044812"}, // valid TLV but no tag 51 subfield 02
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseQRIS(tc.qr); err == nil {
				t.Errorf("ParseQRIS(%q) expected error, got nil", tc.qr)
			}
		})
	}
}

func TestParseQRIS_PakailinkLikeDynamic(t *testing.T) {
	// Sample from Nobu generate doc (dynamic, tag 01 == 12). NMID ID2020081400178.
	const qr = "00020101021226670016COM.NOBUBANK.WWW01189360050300000488910214051200000314150303UMI51440014ID.CO.QRIS.WWW0215ID20200814001780303UMI52045499530336054061750005802ID5912FLORIST LILY6015Banten - Kota T61051581162540114051600001421300617202305161512305480703A010804POSP63046D61"
	info, err := ParseQRIS(qr)
	if err != nil {
		t.Fatalf("ParseQRIS returned error: %v", err)
	}
	if info.NMID != "ID2020081400178" {
		t.Errorf("NMID = %q, want %q", info.NMID, "ID2020081400178")
	}
	if info.TerminalID != "A01" {
		t.Errorf("TerminalID = %q, want %q", info.TerminalID, "A01")
	}
	if info.IsStatic {
		t.Errorf("IsStatic = true, want false (tag 01 == 12)")
	}
}
