package embeddings

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	// onnxEmbeddingDimensions matches EmbeddingsConfig.Dimensions and the
	// storage schema, so the encoder needs no migration.
	onnxEmbeddingDimensions = 384
	onnxModelFileName       = "model.onnx"
	onnxVocabFileName       = "vocab.txt"
	onnxMaxSequenceLength   = 512
	// onnxEmbedBatchSize bounds the memory one session run allocates when a
	// large batch is handed in.
	onnxEmbedBatchSize = 32

	// The BERT special-token ids of the pinned encoder's vocabulary.
	bertTokenUNK int64 = 100
	bertTokenCLS int64 = 101
	bertTokenSEP int64 = 102
)

// ONNXProvider is a local, CPU-only text embedding provider backed by a small
// ONNX encoder loaded in-process. The runtime it loads is the one sherpa-onnx
// already ships beside the binary, so no second runtime is packaged. When the
// runtime or the model is absent the factory falls back to the hash projection.
type ONNXProvider struct {
	id        string
	modelDir  string
	tokenizer *bertTokenizer

	mu      sync.Mutex
	session *ort.DynamicAdvancedSession
	options *ort.SessionOptions
}

// NewONNXProvider loads the encoder from modelDir. It returns an error when the
// runtime cannot be loaded, or the model or its vocabulary is missing or
// unreadable, which the factory turns into a fallback to the hash projection.
func NewONNXProvider(modelDir string) (Provider, error) {
	if modelDir == "" {
		return nil, fmt.Errorf("no model directory for the ONNX embedding provider")
	}
	modelPath := filepath.Join(modelDir, onnxModelFileName)
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("embedding model not found at %s: %w", modelPath, err)
	}
	tokenizer, err := loadBertTokenizer(filepath.Join(modelDir, onnxVocabFileName), onnxMaxSequenceLength)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	if err := ensureONNXRuntime(); err != nil {
		return nil, err
	}

	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("create session options: %w", err)
	}
	// Embedding runs behind the turn; a small pool keeps it from competing with
	// playback and the narrator.
	if err := options.SetIntraOpNumThreads(2); err != nil {
		_ = options.Destroy()
		return nil, fmt.Errorf("set thread count: %w", err)
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		options)
	if err != nil {
		_ = options.Destroy()
		return nil, fmt.Errorf("open embedding model: %w", err)
	}

	return &ONNXProvider{
		id:        "bge-small-en-v1.5",
		modelDir:  modelDir,
		tokenizer: tokenizer,
		session:   session,
		options:   options,
	}, nil
}

// ID identifies the encoder. It is stored with every vector, so changing the
// model must change this string.
func (p *ONNXProvider) ID() string { return p.id }

// Dimensions is the encoder's hidden size.
func (p *ONNXProvider) Dimensions() int { return onnxEmbeddingDimensions }

// Embed returns one L2-normalised vector per input text.
func (p *ONNXProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	if len(texts) == 0 {
		return out, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session == nil {
		return nil, fmt.Errorf("embedding model is not loaded")
	}

	for start := 0; start < len(texts); start += onnxEmbedBatchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+onnxEmbedBatchSize, len(texts))
		if err := p.embedBatch(texts[start:end], out[start:end]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (p *ONNXProvider) embedBatch(texts []string, out [][]float32) error {
	batch := len(texts)
	encoded := make([][]int64, batch)
	seq := 0
	for i, text := range texts {
		encoded[i] = p.tokenizer.encode(text)
		if len(encoded[i]) > seq {
			seq = len(encoded[i])
		}
	}
	if seq == 0 {
		seq = 1
	}

	ids := make([]int64, batch*seq)
	mask := make([]int64, batch*seq)
	types := make([]int64, batch*seq)
	for i, row := range encoded {
		for j, id := range row {
			ids[i*seq+j] = id
			mask[i*seq+j] = 1
		}
	}

	shape := ort.NewShape(int64(batch), int64(seq))
	inputIDs, err := ort.NewTensor(shape, ids)
	if err != nil {
		return fmt.Errorf("build input ids: %w", err)
	}
	defer inputIDs.Destroy()
	attention, err := ort.NewTensor(shape, mask)
	if err != nil {
		return fmt.Errorf("build attention mask: %w", err)
	}
	defer attention.Destroy()
	tokenTypes, err := ort.NewTensor(shape, types)
	if err != nil {
		return fmt.Errorf("build token types: %w", err)
	}
	defer tokenTypes.Destroy()

	output, err := ort.NewEmptyTensor[float32](ort.NewShape(int64(batch), int64(seq), onnxEmbeddingDimensions))
	if err != nil {
		return fmt.Errorf("build output tensor: %w", err)
	}
	defer output.Destroy()

	if err := p.session.Run(
		[]ort.Value{inputIDs, attention, tokenTypes},
		[]ort.Value{output},
	); err != nil {
		return fmt.Errorf("run embedding model: %w", err)
	}

	data := output.GetData()
	for i := range batch {
		// BGE pools the CLS token (position 0) and L2-normalises.
		base := i * seq * onnxEmbeddingDimensions
		vec := make([]float32, onnxEmbeddingDimensions)
		var sumSquares float64
		for d := range onnxEmbeddingDimensions {
			v := data[base+d]
			vec[d] = v
			sumSquares += float64(v) * float64(v)
		}
		if norm := math.Sqrt(sumSquares); norm > 0 {
			for d := range vec {
				vec[d] = float32(float64(vec[d]) / norm)
			}
		}
		out[i] = vec
	}
	return nil
}

var (
	onnxRuntimeOnce sync.Once
	onnxRuntimeErr  error
)

// ensureONNXRuntime loads the ONNX Runtime shared library once per process. The
// library is the one sherpa-onnx already ships, resolved through the loader's
// search path; LOCALRPG_ONNXRUNTIME_LIB overrides the name.
func ensureONNXRuntime() error {
	onnxRuntimeOnce.Do(func() {
		var lastErr error
		for _, candidate := range onnxRuntimeCandidates() {
			ort.SetSharedLibraryPath(candidate)
			if err := ort.InitializeEnvironment(); err != nil {
				lastErr = fmt.Errorf("load ONNX Runtime %q: %w", candidate, err)
				continue
			}
			return
		}
		onnxRuntimeErr = lastErr
	})
	return onnxRuntimeErr
}

func onnxRuntimeCandidates() []string {
	candidates := make([]string, 0, 4)
	if override := strings.TrimSpace(os.Getenv("LOCALRPG_ONNXRUNTIME_LIB")); override != "" {
		candidates = append(candidates, override)
	}
	switch runtime.GOOS {
	case "windows":
		candidates = append(candidates, "onnxruntime.dll")
	case "darwin":
		candidates = append(candidates, "libonnxruntime.dylib")
	default:
		candidates = append(candidates, "libonnxruntime.so", "libonnxruntime.so.1", "onnxruntime.so")
	}
	return candidates
}

// bertTokenizer is a WordPiece tokenizer for BERT-family encoders. It is the
// tokenization the pinned BGE model expects: lowercase, split on whitespace and
// punctuation, then greedy longest-match against the vocabulary.
type bertTokenizer struct {
	vocab  map[string]int64
	maxLen int
}

func loadBertTokenizer(path string, maxLen int) (*bertTokenizer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	t := &bertTokenizer{vocab: make(map[string]int64, 30522), maxLen: maxLen}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var id int64
	for scanner.Scan() {
		t.vocab[scanner.Text()] = id
		id++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(t.vocab) == 0 {
		return nil, fmt.Errorf("tokenizer vocabulary is empty: %s", path)
	}
	return t, nil
}

// encode returns the token ids for text, wrapped in [CLS] and [SEP] and
// truncated to the tokenizer's maximum length.
func (t *bertTokenizer) encode(text string) []int64 {
	ids := make([]int64, 0, 16)
	ids = append(ids, bertTokenCLS)
	for _, word := range basicTokenize(text) {
		for _, piece := range t.wordpiece(word) {
			id, ok := t.vocab[piece]
			if !ok {
				id = bertTokenUNK
			}
			ids = append(ids, id)
		}
	}
	ids = append(ids, bertTokenSEP)
	if len(ids) > t.maxLen {
		ids = append(ids[:t.maxLen-1], bertTokenSEP)
	}
	return ids
}

// wordpiece splits a word into the longest vocabulary prefixes it can, marking
// continuation pieces with the "##" prefix.
func (t *bertTokenizer) wordpiece(word string) []string {
	runes := []rune(word)
	if len(runes) > 100 {
		return []string{"[UNK]"}
	}
	var pieces []string
	for start := 0; start < len(runes); {
		end := len(runes)
		var match string
		found := false
		for start < end {
			piece := string(runes[start:end])
			if start > 0 {
				piece = "##" + piece
			}
			if _, ok := t.vocab[piece]; ok {
				match = piece
				found = true
				break
			}
			end--
		}
		if !found {
			return []string{"[UNK]"}
		}
		pieces = append(pieces, match)
		start = end
	}
	return pieces
}

// basicTokenize lowercases text and splits it on whitespace and punctuation,
// discarding control characters the way the reference BERT tokenizer does.
func basicTokenize(text string) []string {
	text = strings.ToLower(text)
	var out []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			out = append(out, string(current))
			current = current[:0]
		}
	}
	for _, r := range text {
		switch {
		case r == 0 || r == '\uFFFD':
			continue
		case unicode.IsControl(r):
			continue
		case unicode.IsSpace(r):
			flush()
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			flush()
			out = append(out, string(r))
		default:
			current = append(current, r)
		}
	}
	flush()
	return out
}

var _ Provider = (*ONNXProvider)(nil)
