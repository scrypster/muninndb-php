package embed

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestMeanPool verifies mean pooling logic against a hand-calculated reference.
func TestMeanPool(t *testing.T) {
	// 3 tokens, dim=2.  Token 0 and 2 are active (mask=1), token 1 is padding (mask=0).
	hidden := []float32{
		1, 2,   // token 0
		10, 10, // token 1 (padding — excluded)
		3, 4,   // token 2
	}
	mask := []int64{1, 0, 1}

	got := meanPool(hidden, mask, 3, 2)

	// Expected: mean of tokens 0 and 2 = ([1+3]/2, [2+4]/2) = (2, 3)
	if diff := abs32(got[0] - 2.0); diff > 1e-5 {
		t.Errorf("dim 0: want 2.0, got %.6f", got[0])
	}
	if diff := abs32(got[1] - 3.0); diff > 1e-5 {
		t.Errorf("dim 1: want 3.0, got %.6f", got[1])
	}
}

// TestMeanPoolAllPadding verifies that an all-zero mask returns a zero vector without panic.
func TestMeanPoolAllPadding(t *testing.T) {
	hidden := []float32{1, 2, 3, 4}
	mask := []int64{0, 0}
	got := meanPool(hidden, mask, 2, 2)
	if got[0] != 0 || got[1] != 0 {
		t.Errorf("all-padding: expected [0 0], got %v", got)
	}
}

// TestL2Normalize verifies that the output vector has unit norm.
func TestL2Normalize(t *testing.T) {
	v := []float32{3, 4} // L2 norm = 5; expected output [0.6, 0.8]
	l2Normalize(v)

	norm := computeNorm(v)
	if diff := math.Abs(norm - 1.0); diff > 1e-6 {
		t.Errorf("expected unit norm, got %.8f", norm)
	}
	if diff := abs32(v[0] - 0.6); diff > 1e-5 {
		t.Errorf("v[0]: want 0.6, got %.6f", v[0])
	}
	if diff := abs32(v[1] - 0.8); diff > 1e-5 {
		t.Errorf("v[1]: want 0.8, got %.6f", v[1])
	}
}

// TestL2NormalizeZero verifies that a zero vector doesn't panic or produce NaN/Inf.
func TestL2NormalizeZero(t *testing.T) {
	v := []float32{0, 0, 0}
	l2Normalize(v)
	for i, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			t.Errorf("v[%d] is NaN/Inf after zero-norm l2Normalize", i)
		}
	}
}

func TestLocalProvider_Name(t *testing.T) {
	p := &LocalProvider{}
	if p.Name() != "local" {
		t.Errorf("expected name 'local', got %q", p.Name())
	}
}

func TestLocalProvider_MaxBatchSize(t *testing.T) {
	p := &LocalProvider{}
	if p.MaxBatchSize() != localMaxBatch {
		t.Errorf("expected batch size %d, got %d", localMaxBatch, p.MaxBatchSize())
	}
}

func TestLocalProvider_Close_NilSession(t *testing.T) {
	p := &LocalProvider{}
	err := p.Close()
	if err != nil {
		t.Errorf("Close with nil session failed: %v", err)
	}
}

func TestAtomicWrite_Success(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.txt")
	data := []byte("hello world")

	err := atomicWrite(dest, data)
	if err != nil {
		t.Fatalf("atomicWrite failed: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("expected %q, got %q", data, got)
	}
}

func TestAtomicWrite_Overwrite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.txt")

	if err := atomicWrite(dest, []byte("first")); err != nil {
		t.Fatalf("first atomicWrite failed: %v", err)
	}
	if err := atomicWrite(dest, []byte("second")); err != nil {
		t.Fatalf("second atomicWrite failed: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("expected 'second', got %q", got)
	}
}

func TestAtomicWrite_InvalidDir(t *testing.T) {
	err := atomicWrite("/nonexistent/path/file.txt", []byte("data"))
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestReadAll(t *testing.T) {
	data := []byte("test data for readAll")
	got, err := readAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("readAll failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("expected %q, got %q", data, got)
	}
}

func TestReadAll_Empty(t *testing.T) {
	got, err := readAll(bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("readAll failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d bytes", len(got))
	}
}

func TestLocalAvailable(t *testing.T) {
	_ = LocalAvailable()
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func computeNorm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}
