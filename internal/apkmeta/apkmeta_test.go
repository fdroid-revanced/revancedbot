package apkmeta

import "testing"

func TestParseBadging(t *testing.T) {
	const sample = `package: name='com.bandcamp.android' versionCode='220489' versionName='3.3.6' platformBuildVersionName='14' platformBuildVersionCode='34'
sdkVersion:'24'
targetSdkVersion:'34'
`
	info, err := ParseBadging(sample)
	if err != nil {
		t.Fatal(err)
	}
	if info.PackageID != "com.bandcamp.android" {
		t.Fatalf("package %q", info.PackageID)
	}
	if info.VersionName != "3.3.6" {
		t.Fatalf("versionName %q", info.VersionName)
	}
	if info.VersionCode != "220489" {
		t.Fatalf("versionCode %q", info.VersionCode)
	}
}

func TestParseBadging_missing(t *testing.T) {
	if _, err := ParseBadging("nope\n"); err == nil {
		t.Fatal("expected error")
	}
}
