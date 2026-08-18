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

func TestInfo_MatchesRequest(t *testing.T) {
	info := Info{PackageID: "com.example.app", VersionName: "3.3.6", VersionCode: "9"}
	if err := info.MatchesRequest("com.example.app", "3.3.6"); err != nil {
		t.Fatal(err)
	}
	if err := info.MatchesRequest("com.example.app", "3.3"); err != nil {
		t.Fatal(err)
	}
	if err := info.MatchesRequest("com.other.app", "3.3.6"); err == nil {
		t.Fatal("expected package mismatch")
	}
	if err := info.MatchesRequest("com.example.app", "4.0.0"); err == nil {
		t.Fatal("expected version mismatch")
	}
	if err := info.MatchesRequest("", ""); err != nil {
		t.Fatal(err)
	}
}

func TestVersionMatches(t *testing.T) {
	if !VersionMatches("3.3.6", "3.3.6") || !VersionMatches("3.3", "3.3.6") {
		t.Fatal("expected match")
	}
	if VersionMatches("4.0.0", "3.3.6") {
		t.Fatal("expected mismatch")
	}
}
