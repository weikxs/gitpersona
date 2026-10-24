package credential

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestServeFailsWithoutAnswerWhenTheRequestLineIsTooLong(t *testing.T) {
	src := &fakeSource{token: "secret-token"}
	var out bytes.Buffer
	tooLong := "password=" + strings.Repeat("x", 128*1024) + "\n"

	err := Serve(context.Background(), "get", strings.NewReader(tooLong), &out, target, src)

	if err == nil || !strings.Contains(err.Error(), "read credential request") {
		t.Fatalf("Serve() error = %v, want a read error", err)
	}
	if out.Len() != 0 || src.calledTimes != 0 {
		t.Errorf("Serve() wrote %q and asked for a token %d times, want neither", out.String(), src.calledTimes)
	}
}
