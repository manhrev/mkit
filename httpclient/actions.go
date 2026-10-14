package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/avast/retry-go/v4"
)

// Post executes the request as an HTTP POST, decoding a 2xx JSON response
// body into T. Use struct{} when the body doesn't matter.
func (r *Request) Post[T any](ctx context.Context, url string) (*Response[T], error) {
	return send[T](ctx, r, url, http.MethodPost)
}

// Get executes the request as an HTTP GET, decoding a 2xx JSON response
// body into T. Use struct{} when the body doesn't matter.
func (r *Request) Get[T any](ctx context.Context, url string) (*Response[T], error) {
	return send[T](ctx, r, url, http.MethodGet)
}

// send runs the request and decodes the body into T on 2xx. A non-2xx
// status is not an error: the caller checks StatusCode and reads Raw.
func send[T any](ctx context.Context, r *Request, url string, method string) (*Response[T], error) {
	res, raw, err := r.doHTTP(ctx, url, method)
	if err != nil {
		return nil, err
	}

	response := &Response[T]{
		Raw:         raw,
		StatusCode:  res.StatusCode,
		RawResponse: res,
	}

	// empty 2xx (e.g. 204) leaves Body at its zero value
	if res.StatusCode/100 == 2 && len(raw) > 0 {
		if err := json.Unmarshal(raw, &response.Body); err != nil {
			return nil, errors.Join(ErrCannotDecodeResponseBody, err)
		}
	}

	return response, nil
}

func (r *Request) prepareHTTPRequest(ctx context.Context, url string, method string) (*http.Request, error) {
	var bodyReader *bytes.Reader
	if r.requestBody != nil {
		bodyBytes, err := json.Marshal(r.requestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to parse request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	var req *http.Request
	var err error
	if bodyReader != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, bodyReader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, err
	}

	for k, vs := range r.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	q := req.URL.Query()
	for k, v := range r.queries {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return req, nil
}

// doHTTP reads the whole response body before returning: the timeout
// context is cancelled on return, which would break any later read.
func (r *Request) doHTTP(ctx context.Context, url string, method string) (*http.Response, []byte, error) {
	client := r.client.GetHTTPClient()
	rCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	req, err := r.prepareHTTPRequest(rCtx, url, method)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init http request: %w", err)
	}

	res, err := doWithRetry(client, req, r.retryCount)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, ErrRequestTimeout
		}
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return res, raw, nil
}

// doWithRetry runs req, retrying on transient network errors and on
// 502/503/504 responses (see shouldRetry) up to retryCount times.
// retryCount <= 0 means no retries. If every attempt gets a retryable
// status, the last response is returned without error.
func doWithRetry(client *http.Client, req *http.Request, retryCount int) (*http.Response, error) {
	attempts := uint(1)
	if retryCount > 0 {
		attempts = uint(retryCount)
	}

	var res *http.Response
	err := retry.Do(
		func() error {
			if res != nil {
				// previous attempt got a retryable status; drop it
				res.Body.Close()
				res = nil
			}
			if req.GetBody != nil {
				// the previous attempt consumed req.Body; rewind it
				body, err := req.GetBody()
				if err != nil {
					return retry.Unrecoverable(err)
				}
				req.Body = body
			}

			var err error
			res, err = client.Do(req)
			if shouldRetry(err, res) {
				return errors.Join(errStatusCodeShouldRetry, err)
			}
			return err
		},
		retry.RetryIf(func(err error) bool {
			return errors.Is(err, errStatusCodeShouldRetry)
		}),
		retry.Attempts(attempts),
		retry.Delay(3*time.Second),
		retry.MaxJitter(1*time.Second),
		retry.LastErrorOnly(true),
	)

	switch {
	case err == nil, res != nil:
		// res != nil with an error = retryable status until max attempts
		return res, nil
	case errors.Is(err, context.DeadlineExceeded):
		return nil, ErrRequestTimeout
	default:
		return nil, fmt.Errorf("error while making http request: %w", err)
	}
}

// shouldRetry reports whether a failed attempt (network error or response)
// is worth retrying: any error other than context cancellation/deadline, or
// a 502/503/504 response.
func shouldRetry(err error, resp *http.Response) bool {
	if err != nil {
		return !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
	}

	if resp.StatusCode == http.StatusBadGateway ||
		resp.StatusCode == http.StatusServiceUnavailable ||
		resp.StatusCode == http.StatusGatewayTimeout {
		return true
	}
	return false
}
