package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var errExtraJSONValues = errors.New("request body must contain exactly one JSON value")

type jsonBodyDecodeError struct {
	err  error
	body string
}

func (e *jsonBodyDecodeError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *jsonBodyDecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func decodeJSONBodySingle(r *http.Request, dst any, allowEmpty bool) error {
	return decodeJSONBody(r, dst, allowEmpty, false)
}

func decodeJSONBodyStrict(r *http.Request, dst any, allowEmpty bool) error {
	return decodeJSONBody(r, dst, allowEmpty, true)
}

func decodeJSONBody(r *http.Request, dst any, allowEmpty bool, disallowUnknownFields bool) error {
	if r == nil || r.Body == nil {
		if allowEmpty {
			return nil
		}
		return io.EOF
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		return &jsonBodyDecodeError{err: err, body: ""}
	}
	bodyText := string(rawBody)

	dec := json.NewDecoder(bytes.NewReader(rawBody))

	if disallowUnknownFields {
		dec.DisallowUnknownFields()
	}

	if err := dec.Decode(dst); err != nil {
		if allowEmpty && errors.Is(err, io.EOF) {
			return nil
		}
		return &jsonBodyDecodeError{err: err, body: bodyText}
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return &jsonBodyDecodeError{err: errExtraJSONValues, body: bodyText}
		}
		return &jsonBodyDecodeError{err: fmt.Errorf("invalid trailing JSON: %w", err), body: bodyText}
	}
	return nil
}
