package releaseidentity

import "testing"

func TestParse(t *testing.T) {
	sha := "12f83b5248253662d8af06030e3966034a7ef5d4"
	for _, tag := range []string{"v1.233.0", "v1.233.1-rc.0", "v1.234.0-test.1"} {
		version, commit, err := Parse("atmos-release:" + tag + ":" + sha + ":end")
		if err != nil || version != tag[1:] || commit != sha {
			t.Fatalf("Parse(%s) = %q, %q, %v", tag, version, commit, err)
		}
	}
	for _, invalid := range []string{"", "v1.233.0", "atmos-release:v1.233.0:short:end", "atmos-release:v1.233.0:" + sha, "atmos-release:main:" + sha + ":end"} {
		if _, _, err := Parse(invalid); err == nil {
			t.Fatalf("accepted invalid identity %q", invalid)
		}
	}
}
