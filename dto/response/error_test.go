package response

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/manhrev/mkit/error/serviceerr"
)

func TestNewErrorHidesNonServiceError(t *testing.T) {
	out := NewError(context.Background(), errors.New(`pq: duplicate key violates "users_email_key"`)).(*ErrorOutput)
	if out.Status != http.StatusInternalServerError || out.Detail != "Internal server error." {
		t.Fatalf("got %d %q, want 500 with a generic detail", out.Status, out.Detail)
	}

	out = NewError(context.Background(), serviceerr.NewNotFound(errors.New("x")).SetMessage("Share not found.")).(*ErrorOutput)
	if out.Status != http.StatusNotFound || out.Detail != "Share not found." {
		t.Fatalf("got %d %q, want the service error's status and message", out.Status, out.Detail)
	}
}
