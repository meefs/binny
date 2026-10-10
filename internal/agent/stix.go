package agent

// STIX 2.1 malware-type-ov (OASIS §10.15). Open vocabulary, NOT mutually
// exclusive — a sample can be both `spyware` and `screen-capture`, so
// classification is multi-label. Triage proposes one or more of these; the
// type→playbook map (playbook package) selects which playbook(s) to run.

// STIXMalwareTypes is the preferred vocabulary. `unknown` is used when triage
// can't determine a type. The vocab is open (a novel type may be added) but
// BINNY should prefer these.
var STIXMalwareTypes = []string{
	"adware", "backdoor", "bot", "bootkit", "ddos", "downloader", "dropper",
	"exploit-kit", "keylogger", "ransomware", "remote-access-trojan",
	"resource-exploitation", "rogue-security-software", "rootkit",
	"screen-capture", "spyware", "trojan", "unknown", "virus", "webshell",
	"wiper", "worm",
}

// IsSTIXType reports whether t is in the preferred vocabulary.
func IsSTIXType(t string) bool {
	for _, s := range STIXMalwareTypes {
		if s == t {
			return true
		}
	}
	return false
}
