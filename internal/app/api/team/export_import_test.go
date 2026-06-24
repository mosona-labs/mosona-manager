package ateam

import (
	"encoding/json"
	"strings"
	"testing"

	"mosona-manager/internal/security/exportcrypto"
)

func TestDecodeTeamImportBundleEncrypted(t *testing.T) {
	bundle := testTeamExportBundle()
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", bundle)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "export-pass-123",
		Encrypted:      encrypted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != teamExportVersion || got.Team.Name != bundle.Team.Name {
		t.Fatalf("got %#v", got)
	}
}

func TestDecodeTeamImportBundlePlaintextIgnoresPassword(t *testing.T) {
	bundle := testTeamExportBundle()
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	got, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "short",
		Data:           raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != teamExportVersion || got.Team.Name != bundle.Team.Name {
		t.Fatalf("got %#v", got)
	}
}

func TestDecodeTeamImportBundleDataTakesPriorityOverEncrypted(t *testing.T) {
	plaintext := testTeamExportBundle()
	plaintext.Team.Name = "From Plaintext Data"
	raw, err := json.Marshal(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	other := testTeamExportBundle()
	other.Team.Name = "From Encrypted"
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", other)
	if err != nil {
		t.Fatal(err)
	}

	got, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "wrong-password",
		Encrypted:      encrypted,
		Data:           raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Team.Name != "From Plaintext Data" {
		t.Fatalf("expected plaintext branch, got %#v", got)
	}
}

func TestDecodeTeamImportBundleMissingBoth(t *testing.T) {
	_, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "export-pass-123",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing encrypted export payload") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeTeamImportBundleNullDataUsesEncrypted(t *testing.T) {
	bundle := testTeamExportBundle()
	bundle.Team.Name = "From Encrypted Only"
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", bundle)
	if err != nil {
		t.Fatal(err)
	}

	got, err := decodeTeamImportBundle(teamImportRequest{
		ExportPassword: "export-pass-123",
		Encrypted:      encrypted,
		Data:           json.RawMessage("null"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Team.Name != "From Encrypted Only" {
		t.Fatalf("expected encrypted branch, got %#v", got)
	}
}

func TestDecodeTeamImportBundleWhitespaceDataUsesEncrypted(t *testing.T) {
	bundle := testTeamExportBundle()
	encrypted, err := exportcrypto.EncryptJSON("export-pass-123", bundle)
	if err != nil {
		t.Fatal(err)
	}

	for _, raw := range []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("   \n\t")} {
		got, err := decodeTeamImportBundle(teamImportRequest{
			ExportPassword: "export-pass-123",
			Encrypted:      encrypted,
			Data:           raw,
		})
		if err != nil {
			t.Fatalf("data=%q: %v", raw, err)
		}
		if got.Team.Name != bundle.Team.Name {
			t.Fatalf("data=%q: got %#v", raw, got)
		}
	}
}

func testTeamExportBundle() teamExportBundle {
	return teamExportBundle{
		Version: teamExportVersion,
		Team: teamExportTeam{
			Name:        "Legacy Team",
			Description: "imported from plaintext export",
			Color:       "#2563eb",
			Image:       "",
		},
		Categories: []teamExportCategory{
			{RefID: 1, Name: "Default", Sort: 0},
		},
		Servers: []teamExportServer{},
	}
}