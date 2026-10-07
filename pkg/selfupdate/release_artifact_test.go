package selfupdate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Test-only: use exactly the production updater's embedded public key, not
// another verification dependency or the signing step's private credential.
func verifyReleaseArtifactsForTest(directory string) error {
	read := func(name string) ([]byte, error) {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, fmt.Errorf("release artifact %s must be a nonempty regular file", name)
		}
		return os.ReadFile(path)
	}
	manifest, err := read("checksums.txt")
	if err != nil {
		return err
	}
	signature, err := read("checksums.txt.sig")
	if err != nil {
		return err
	}
	if err := verifyManifestSignature(manifest, signature); err != nil {
		return err
	}
	for _, name := range []string{"pith-linux-amd64", "pith-windows-amd64.exe", "pith-darwin-arm64"} {
		expected, err := checksumForAsset(manifest, name)
		if err != nil {
			return err
		}
		binary, err := read(name)
		if err != nil {
			return err
		}
		if actual := fmt.Sprintf("%x", sha256.Sum256(binary)); actual != expected {
			return fmt.Errorf("release artifact %s checksum mismatch", name)
		}
	}
	return nil
}

// Normal source CI cannot attest to a signature that has not been generated.
// The existing tag-push release job explicitly requests this test after signing;
// count=1 is required there so an earlier skip cannot satisfy the release gate.
func TestReleaseArtifactSignature(t *testing.T) {
	directory, requested := os.LookupEnv("PITH_TEST_RELEASE_DIST")
	if !requested {
		t.Skip("signed candidate only exists in the tag-push release job")
	}
	if directory == "" {
		t.Fatal("PITH_TEST_RELEASE_DIST was requested but is empty")
	}
	if err := verifyReleaseArtifactsForTest(directory); err != nil {
		t.Fatal(err)
	}
	t.Log("Verified candidate manifest with embedded production public key and all three binary hashes")
}

func TestReleaseArtifactsRejectInvalidInput(t *testing.T) {
	directory := t.TempDir()
	if err := verifyReleaseArtifactsForTest(directory); err == nil {
		t.Fatal("missing manifest accepted")
	}
	manifest := []byte("synthetic manifest\n")
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseArtifactsForTest(directory); err == nil {
		t.Fatal("missing signature accepted")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifest)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, []byte("not-base64"), []byte(base64.StdEncoding.EncodeToString(signature))} {
		if err := os.WriteFile(filepath.Join(directory, "checksums.txt.sig"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := verifyReleaseArtifactsForTest(directory); err == nil {
			t.Fatal("empty, malformed, or wrong-key signature accepted")
		}
	}
}
