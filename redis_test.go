package cache_redis

import "testing"

func TestRedisScanPatternEscapesGlobCharacters(t *testing.T) {
	want := `a\[1\]\?\*\\*`
	if got := redisScanPattern("a[1]?*\\"); got != want {
		t.Fatalf("unexpected scan pattern: %q want %q", got, want)
	}
	if got := redisScanPattern(""); got != "*" {
		t.Fatalf("empty prefix pattern: %q", got)
	}
}

func TestIntSettingAcceptsDecodedNumbers(t *testing.T) {
	got, ok := intSetting(float64(2))
	if !ok || got != 2 {
		t.Fatalf("float database setting: %d %v", got, ok)
	}
	got, ok = intSetting("3")
	if !ok || got != 3 {
		t.Fatalf("string database setting: %d %v", got, ok)
	}
}
