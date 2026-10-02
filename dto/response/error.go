package response

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/manhrev/mkit/error/serviceerr"
	applog "github.com/manhrev/mkit/log"
)

// ErrorOutput is huma's own ErrorModel (type/title/status/detail/errors,
// unchanged shape/keys) plus a top-level "meta" field. *huma.ErrorModel is
// embedded so its fields flatten into the same JSON object as meta (no
// nested "error" key), and its Error()/GetStatus()/ContentType() methods
// promote automatically — ErrorOutput satisfies huma's StatusError as-is.
type ErrorOutput struct {
	Meta Meta `json:"meta"`
	*huma.ErrorModel
}

// NewError logs err (Error level, full underlying detail — the exact error
// the service returned, not just the HTTP status huma derives from it)
// when it's a real failure — a 5xx, or an error that isn't a
// *serviceerr.Error at all — then wraps it into an *ErrorOutput and
// returns that, so callers write:
//
//	return nil, response.NewError(ctx, err)
//
// The wrapping matters, not just the logging: huma writes a returned error
// directly as the JSON response body if it implements StatusError (which
// *serviceerr.Error does), but every field on *serviceerr.Error is
// unexported by design — so without this, the client gets `{}` with the
// right status code and nothing else, and no Meta either way (huma's own
// *ErrorModel has no Meta field). ErrorOutput fixes both.
//
// Expected client errors (400/404/409/...) are not logged; huma still
// renders the correct status/body for them either way.
func NewError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	sErr, isServiceErr := errors.AsType[*serviceerr.Error](err)

	model := toHumaErrorModel(err, sErr, isServiceErr)

	if model.Status >= http.StatusInternalServerError {
		args := []any{"error", err}
		if isServiceErr {
			if details := sErr.Details(); len(details) > 0 {
				args = append(args, "details", details)
			}
		}
		applog.FromContext(ctx).ErrorContext(ctx, "handler error", args...)
	}

	meta, _ := MetaFromContext(ctx)
	meta.ResponseAt = time.Now()

	return &ErrorOutput{Meta: meta, ErrorModel: model}
}

// toHumaErrorModel converts err into huma's own ErrorModel (exported,
// properly json-tagged) so its message/details are actually serialized.
// *serviceerr.Error carries a real status/message/details; anything else
// falls back to a plain 500 with err's own message.
func toHumaErrorModel(err error, sErr *serviceerr.Error, isServiceErr bool) *huma.ErrorModel {
	if !isServiceErr {
		return &huma.ErrorModel{
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: err.Error(),
		}
	}

	status := sErr.GetStatus()
	model := &huma.ErrorModel{
		Title:  http.StatusText(status),
		Status: status,
		Detail: sErr.Message(),
	}

	for _, d := range sErr.Details() {
		model.Errors = append(model.Errors, &huma.ErrorDetail{
			Message:  d.Message,
			Location: "body." + d.Field,
			Value:    d.Code, // huma.ErrorDetail has no dedicated code field
		})
	}

	return model
}
