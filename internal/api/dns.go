package api

import "strings"

// ParseDNSRecords maps the raw positional dns_records array returned by
// POST /api/domains into a named DNSRecords struct.
//
// The API returns exactly three records today (DKIM, SPF, DMARC), but their
// order is not guaranteed by contract. Matching is content-based, never
// position-based, so reordering or the future addition of new record types
// cannot silently mis-wire SPF/data into DKIM slots.
//
// Records that match none of the three known forms are collected into
// DNSRecords.Unrecognized and surfaced by the caller — an unexpected record is
// reported, never dropped.
func ParseDNSRecords(raw []DNSRecord) DNSRecords {
	out := DNSRecords{}
	for _, r := range raw {
		switch classifyDNSRecord(r) {
		case dkimRecord:
			out.DKIM = r
		case spfRecord:
			out.SPF = r
		case dmarcRecord:
			out.DMARC = r
		default:
			out.Unrecognized = append(out.Unrecognized, UnknownRecord{
				Type: r.Type, Host: r.Host, Value: r.Value,
			})
		}
	}
	return out
}

type dnsRecordKind int

const (
	unknownRecord dnsRecordKind = iota
	dkimRecord
	spfRecord
	dmarcRecord
)

// classifyDNSRecord decides which of the three record kinds a raw record is.
func classifyDNSRecord(r DNSRecord) dnsRecordKind {
	host := strings.ToLower(strings.TrimSuffix(r.Host, "."))
	value := strings.ToLower(strings.TrimSpace(r.Value))

	switch {
	case strings.Contains(host, "_domainkey.") && strings.HasPrefix(value, "v=dkim1"):
		return dkimRecord
	case strings.HasPrefix(host, "_dmarc.") && strings.HasPrefix(value, "v=dmarc1"):
		return dmarcRecord
	case strings.HasPrefix(value, "v=spf1"):
		// SPF lives at the apex; never classify a prefixed host as SPF without
		// proof it is the domain root. Fall through to unknown otherwise.
		if !strings.Contains(host, ".") || hostIsApex(host) {
			return spfRecord
		}
		return unknownRecord
	default:
		return unknownRecord
	}
}

// hostIsApex reports whether host looks like an apex (domain root) rather than
// a DNS label prefix. The API returns full FQDNs (e.g. "example.com"), so a
// host with exactly one dot is treated as a bare apex.
func hostIsApex(host string) bool {
	n := strings.Count(host, ".")
	switch n {
	case 0:
		return true
	case 1:
		// e.g. "co.uk" vs "example.com" — both single-dot forms are apex-like
		// for our purpose; DKIM/DMARC prefixes never reach this branch.
		return true
	default:
		return false
	}
}
