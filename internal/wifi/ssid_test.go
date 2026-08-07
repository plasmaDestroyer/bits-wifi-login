package wifi

import "testing"

func TestParseNmcli(t *testing.T) {
	// Captured from a real `nmcli -t -f active,ssid dev wifi` on campus — note the
	// active row is last, and hidden networks show up as empty-SSID rows.
	const real = `no:BITS-STAFF
no:BITS-STUDENT
no:
no:VFAST
no:
no:BITS-STUDENT
no:VFAST
yes:BITS-STAFF
`

	cases := []struct {
		name string
		out  string
		want string
	}{
		{"real capture", real, "BITS-STAFF"},
		{"first row active", "yes:BITS-STUDENT\nno:VFAST\n", "BITS-STUDENT"},
		{"not associated", "no:BITS-STAFF\nno:VFAST\n", ""},
		{"empty output", "", ""},
		{"escaped colon in ssid", `yes:Guest\:Net`, "Guest:Net"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseNmcli(c.out); got != c.want {
				t.Errorf("parseNmcli() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseHardwarePorts(t *testing.T) {
	const out = `Hardware Port: Ethernet
Device: en1
Ethernet Address: aa:bb:cc:dd:ee:ff

Hardware Port: Wi-Fi
Device: en0
Ethernet Address: 11:22:33:44:55:66
`

	if got := parseHardwarePorts(out); got != "en0" {
		t.Errorf("parseHardwarePorts() = %q, want %q", got, "en0")
	}
	if got := parseHardwarePorts("Hardware Port: Ethernet\nDevice: en1\n"); got != "" {
		t.Errorf("parseHardwarePorts() with no Wi-Fi port = %q, want empty", got)
	}
}

func TestParseAirport(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{"associated", "Current Wi-Fi Network: BITS-STUDENT\n", "BITS-STUDENT"},
		{"not associated", "You are not associated with an AirPort network.\n", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseAirport(c.out); got != c.want {
				t.Errorf("parseAirport() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseNetsh(t *testing.T) {
	// BSSID sits right below SSID and its value looks nothing like a network name —
	// matching it instead is the obvious way to get this wrong.
	const out = "\r\nThere is 1 interface on the system:\r\n\r\n" +
		"    Name                   : Wi-Fi\r\n" +
		"    State                  : connected\r\n" +
		"    SSID                   : BITS-STUDENT\r\n" +
		"    BSSID                  : 00:11:22:33:44:55\r\n" +
		"    Signal                 : 87%\r\n"

	cases := []struct {
		name string
		out  string
		want string
	}{
		{"connected", out, "BITS-STUDENT"},
		{"disconnected", "    State                  : disconnected\r\n", ""},
		{"empty output", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseNetsh(c.out); got != c.want {
				t.Errorf("parseNetsh() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestIsBITS(t *testing.T) {
	cases := []struct {
		ssid string
		want bool
	}{
		{"BITS-STUDENT", true},
		{"BITS-STAFF", true},
		{"BITS-GUEST", false},
		{"VFAST", false},
		{"", false},
		{"xBITS-STAFF", false},
		{"BITS-STAFF ", false},
	}

	for _, c := range cases {
		if got := IsBITS(c.ssid); got != c.want {
			t.Errorf("IsBITS(%q) = %v, want %v", c.ssid, got, c.want)
		}
	}
}
