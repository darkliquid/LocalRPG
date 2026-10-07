package embeddings

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// testModelDir returns the directory holding the pinned encoder, or skips the
// test when it is not installed. It honours LOCALRPG_EMBEDDING_MODEL_DIR so a
// developer can point it at a downloaded copy, and otherwise looks where the
// model manager installs the encoder.
func testModelDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("LOCALRPG_EMBEDDING_MODEL_DIR")
	if dir == "" {
		if cache, err := os.UserCacheDir(); err == nil {
			dir = filepath.Join(cache, "localrpg", "models", "embeddings", "bge-small-en-v1.5")
		}
	}
	if dir == "" {
		t.Skip("embedding model directory is not configured")
	}
	for _, name := range []string{onnxModelFileName, onnxVocabFileName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Skipf("embedding model is not installed in %s", dir)
		}
	}
	return dir
}

func TestONNXProviderIsDeterministic(t *testing.T) {
	p, err := NewONNXProvider(testModelDir(t))
	if err != nil {
		t.Skipf("embedding runtime unavailable: %v", err)
	}

	a, err := p.Embed(context.Background(), []string{"a sword"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	b, err := p.Embed(context.Background(), []string{"a sword"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(a) != 1 || len(a[0]) != onnxEmbeddingDimensions {
		t.Fatalf("unexpected shape: %dx%d", len(a), len(a[0]))
	}
	if a[0][0] != b[0][0] {
		t.Fatalf("embedding is not deterministic: %v %v", a[0][0], b[0][0])
	}
	if p.Dimensions() != onnxEmbeddingDimensions {
		t.Fatalf("Dimensions = %d, want %d", p.Dimensions(), onnxEmbeddingDimensions)
	}
	if p.ID() == "" {
		t.Fatal("provider ID is empty")
	}
}

func TestONNXProviderEmbedBatch(t *testing.T) {
	p, err := NewONNXProvider(testModelDir(t))
	if err != nil {
		t.Skipf("embedding runtime unavailable: %v", err)
	}
	texts := []string{"a blade", "the knight drew his sword", "a cabbage", ""}
	vecs, err := p.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(vecs), len(texts))
	}
	for i, v := range vecs {
		if len(v) != onnxEmbeddingDimensions {
			t.Fatalf("vector %d has %d dimensions", i, len(v))
		}
	}
}

func TestONNXProviderMissingModelErrors(t *testing.T) {
	if _, err := NewONNXProvider(t.TempDir()); err == nil {
		t.Fatal("expected an error for a missing model")
	}
	if _, err := NewONNXProvider(""); err == nil {
		t.Fatal("expected an error for an empty model directory")
	}
}

func TestBertTokenizerWrapsAndSplits(t *testing.T) {
	tok := &bertTokenizer{
		vocab: map[string]int64{
			"a": 5, "blade": 6, "##s": 7, "sword": 8,
			"[CLS]": bertTokenCLS, "[SEP]": bertTokenSEP, "[UNK]": bertTokenUNK,
		},
		maxLen: 512,
	}

	ids := tok.encode("a blade")
	if len(ids) < 2 || ids[0] != bertTokenCLS || ids[len(ids)-1] != bertTokenSEP {
		t.Fatalf("encode did not wrap in [CLS]/[SEP]: %v", ids)
	}
	if ids[1] != 5 || ids[2] != 6 {
		t.Fatalf("unexpected word ids: %v", ids)
	}

	// "blades" splits into a known stem plus the "##s" continuation.
	sub := tok.encode("blades")
	if len(sub) != 4 || sub[1] != 6 || sub[2] != 7 {
		t.Fatalf("wordpiece split failed: %v", sub)
	}

	// An unknown word becomes a single [UNK].
	unk := tok.encode("xyzzy")
	if len(unk) != 3 || unk[1] != bertTokenUNK {
		t.Fatalf("unknown word was not mapped to [UNK]: %v", unk)
	}
}

func TestBertTokenizerTruncates(t *testing.T) {
	vocab := map[string]int64{"[CLS]": bertTokenCLS, "[SEP]": bertTokenSEP, "[UNK]": bertTokenUNK}
	for i := range 50 {
		vocab[string(rune('a'+i%26))+string(rune('0'+i/26))] = int64(100 + i)
	}
	tok := &bertTokenizer{vocab: vocab, maxLen: 10}
	ids := tok.encode("a0 a1 a2 a3 a4 a5 a6 a7 a8 a9 b0 b1")
	if len(ids) != 10 {
		t.Fatalf("encode returned %d ids, want the max length 10", len(ids))
	}
	if ids[len(ids)-1] != bertTokenSEP {
		t.Fatalf("truncated sequence does not end with [SEP]: %v", ids)
	}
}
