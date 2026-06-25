package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestServeRejectsBadPort(t *testing.T) {
	d := testDeps(t)
	var out, errOut bytes.Buffer
	// Port 0 with our flag parsing is invalid usage in our command (we require >0).
	if code := run(context.Background(), []string{"serve", "--port", "-1"}, d, &out, &errOut); code == 0 {
		t.Fatal("expected non-zero exit for invalid port")
	}
	if !strings.Contains(errOut.String(), "port") {
		t.Fatalf("stderr = %q, want it to mention port", errOut.String())
	}
}
