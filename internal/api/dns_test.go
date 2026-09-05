package api

import "testing"

func dnsRecord(typ, host, value string) DNSRecord {
	return DNSRecord{Type: typ, Host: host, Value: value}
}

func TestParseDNSRecords_NormalOrder(t *testing.T) {
	got := ParseDNSRecords([]DNSRecord{
		dnsRecord("TXT", "mail._domainkey.example.com", "v=DKIM1; k=rsa; p=ABC"),
		dnsRecord("TXT", "example.com", "v=spf1 ip4:1.2.3.4 -all"),
		dnsRecord("TXT", "_dmarc.example.com", "v=DMARC1; p=quarantine"),
	})
	if got.DKIM.Value == "" || got.SPF.Value == "" || got.DMARC.Value == "" {
		t.Fatalf("expected all three records parsed: %+v", got)
	}
	if len(got.Unrecognized) != 0 {
		t.Fatalf("expected no unrecognized, got %+v", got.Unrecognized)
	}
}

func TestParseDNSRecords_Reordered(t *testing.T) {
	// DMARC first, SPF second, DKIM last — must still land in the named slots.
	got := ParseDNSRecords([]DNSRecord{
		dnsRecord("TXT", "_dmarc.example.com", "v=DMARC1; p=reject"),
		dnsRecord("TXT", "example.com", "v=spf1 a mx -all"),
		dnsRecord("TXT", "mail._domainkey.example.com", "v=DKIM1; k=rsa; p=DEF"),
	})
	if got.DKIM.Host != "mail._domainkey.example.com" || got.SPF.Value != "v=spf1 a mx -all" || got.DMARC.Host != "_dmarc.example.com" {
		t.Fatalf("reordered parse mis-wired: %+v", got)
	}
}

// TestParseDNSRecords_ExtraRecordSurfaced is the critical non-silent-drop
// contract: a fourth, unknown record type must be surfaced in Unrecognized, not
// omitted and not hard-failed.
func TestParseDNSRecords_ExtraRecordSurfaced(t *testing.T) {
	got := ParseDNSRecords([]DNSRecord{
		dnsRecord("TXT", "mail._domainkey.example.com", "v=DKIM1; k=rsa; p=ABC"),
		dnsRecord("TXT", "example.com", "v=spf1 -all"),
		dnsRecord("TXT", "_dmarc.example.com", "v=DMARC1; p=quarantine"),
		dnsRecord("MX", "example.com", "10 mx.mailafrica.online"),
		dnsRecord("TXT", "marketing.example.com", "google-site-verification=zzz"),
	})
	if len(got.Unrecognized) != 2 {
		t.Fatalf("unrecognized = %d, want 2 surfaced", len(got.Unrecognized))
	}
	found := map[string]bool{}
	for _, u := range got.Unrecognized {
		found[u.Host] = true
	}
	if !found["example.com"] || !found["marketing.example.com"] {
		t.Fatalf("unknown records not both surfaced: %+v", got.Unrecognized)
	}
	// Known records still populated.
	if got.DKIM.Value == "" || got.SPF.Value == "" || got.DMARC.Value == "" {
		t.Fatalf("known records should still parse: %+v", got)
	}
}

func TestParseDNSRecords_UppercaseValues(t *testing.T) {
	// API values are typically already lowercase, but classify defensively.
	got := ParseDNSRecords([]DNSRecord{
		dnsRecord("TXT", "mail._domainkey.example.com", "v=DKIM1; k=rsa; p=GHI"),
		dnsRecord("TXT", "example.com", "V=SPF1 -all"),
	})
	if got.SPF.Value != "V=SPF1 -all" {
		t.Fatalf("uppercase SPF not classified: %+v", got.SPF)
	}
}

func TestParseDNSRecords_Empty(t *testing.T) {
	got := ParseDNSRecords(nil)
	if got.DKIM != (DNSRecord{}) || got.SPF != (DNSRecord{}) || got.DMARC != (DNSRecord{}) {
		t.Fatalf("empty input should yield zero struct: %+v", got)
	}
	if len(got.Unrecognized) != 0 {
		t.Fatalf("empty input should have no unrecognized: %+v", got.Unrecognized)
	}
}
