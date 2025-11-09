package sha256

import "testing"

func TestHasherHashDeterministic(t *testing.T) {
	t.Parallel()

	h := New()
	got, err := h.Hash([]byte("hello world"))
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	want := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
	again, err := h.Hash([]byte("hello world"))
	if err != nil {
		t.Fatalf("Hash() repeat error = %v", err)
	}
	if again != got {
		t.Fatalf("expected deterministic hash, got %s vs %s", got, again)
	}
}
