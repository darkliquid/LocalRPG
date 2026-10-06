package provider

import "testing"

func TestParseKeyAcceptsAdapterAndInstance(t *testing.T) {
	tests := []struct {
		in       string
		family   Family
		adapter  string
		instance string
		parent   string
	}{
		{"llm:openaichat", FamilyLLM, "openaichat", "", "llm:openaichat"},
		{"tts:http@localhost:8880", FamilyTTS, "http", "localhost:8880", "tts:http"},
		{"embedding:gemini@default", FamilyEmbedding, "gemini", "default", "embedding:gemini"},
	}
	for _, tc := range tests {
		k, err := ParseKey(tc.in)
		if err != nil {
			t.Fatalf("ParseKey(%q): %v", tc.in, err)
		}
		if k.Family() != tc.family || k.Adapter() != tc.adapter {
			t.Errorf("ParseKey(%q) = %s/%s, want %s/%s", tc.in, k.Family(), k.Adapter(), tc.family, tc.adapter)
		}
		got, ok := k.Instance()
		if ok != (tc.instance != "") || got != tc.instance {
			t.Errorf("ParseKey(%q) instance = %q/%v, want %q", tc.in, got, ok, tc.instance)
		}
		if string(k.Parent()) != tc.parent {
			t.Errorf("ParseKey(%q).Parent() = %q, want %q", tc.in, k.Parent(), tc.parent)
		}
	}
}

func TestParseKeyRejectsMalformed(t *testing.T) {
	for _, in := range []string{"", "llm", ":gemini", "llm:", "LLM:gemini", "llm:Gemini", "llm:gemini@@", "llm:gemini@", "tts:http@Localhost"} {
		if _, err := ParseKey(in); err == nil {
			t.Errorf("ParseKey(%q) = nil error, want rejection", in)
		}
	}
}

func TestInstanceOrSelf(t *testing.T) {
	base, err := NewKey(FamilyTTS, "http")
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	if got := InstanceOrSelf(base, ""); got != base {
		t.Errorf("InstanceOrSelf with no discriminator = %q, want %q", got, base)
	}
	inst := InstanceOrSelf(base, "localhost:8880")
	if inst != "tts:http@localhost:8880" {
		t.Errorf("InstanceOrSelf = %q, want tts:http@localhost:8880", inst)
	}
	if inst.Parent() != base {
		t.Errorf("instance parent = %q, want %q", inst.Parent(), base)
	}
}

func TestInstanceDiscriminator(t *testing.T) {
	if got := InstanceDiscriminator("", "localhost:8880"); got != "localhost:8880" {
		t.Fatalf("derived = %q", got)
	}
	if got := InstanceDiscriminator("narrator", "localhost:8880"); got != "narrator" {
		t.Fatalf("instance = %q", got)
	}
	if got := InstanceDiscriminator("  ", "x"); got != "x" {
		t.Fatalf("blank instance should fall back, got %q", got)
	}
}

func TestDiscriminators(t *testing.T) {
	if got := HostDiscriminator("http://localhost:11434/v1"); got != "localhost:11434" {
		t.Errorf("HostDiscriminator = %q, want localhost:11434", got)
	}
	if got := HostDiscriminator("not a url"); got == "" {
		t.Errorf("HostDiscriminator should still name an odd endpoint")
	}
	if got := HostDiscriminator(""); got != "" {
		t.Errorf("HostDiscriminator(\"\") = %q, want empty", got)
	}
	if got := CommandDiscriminator("/usr/bin/piper"); got != "piper" {
		t.Errorf("CommandDiscriminator = %q, want piper", got)
	}
}
