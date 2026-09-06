package crypto

import "testing"

// TestCiphertextOverheadIsTheTag pins crypto.Overhead to what the ciphers actually add,
// in both modes — the number the repair-bounty geometry (a shard is a whole ciphertext
// chunk) and the publish warning derive from.
func TestCiphertextOverheadIsTheTag(t *testing.T) {
	frame := make([]byte, 1024)
	ct, _, err := ConvergentEncrypt(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(ct)-len(frame) != Overhead {
		t.Fatalf("convergent overhead %d, want %d", len(ct)-len(frame), Overhead)
	}
	var key [KeySize]byte
	pt, err := PrivateEncrypt(key, 7, frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(pt)-len(frame) != Overhead {
		t.Fatalf("private overhead %d, want %d", len(pt)-len(frame), Overhead)
	}
}
