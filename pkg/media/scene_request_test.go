package media

import (
	"context"
	"testing"
)

type fakeHint struct{ got SceneRequest }

func (f *fakeHint) GenerateImage(context.Context, string) ([]byte, error) { return nil, nil }
func (f *fakeHint) GenerateScene(_ context.Context, req SceneRequest) ([]byte, error) {
	f.got = req
	return []byte("<svg></svg>"), nil
}

func TestSceneHintProviderIsDiscoverable(t *testing.T) {
	var c ImageClient = &fakeHint{}
	hp, ok := c.(SceneHintProvider)
	if !ok {
		t.Fatal("expected the fake to implement SceneHintProvider")
	}
	if _, err := hp.GenerateScene(context.Background(), SceneRequest{Genre: "fantasy"}); err != nil {
		t.Fatal(err)
	}
}
