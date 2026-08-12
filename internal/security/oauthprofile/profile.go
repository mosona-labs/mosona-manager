package oauthprofile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxUserInfoBytes = 1 << 20
	maxSubjectBytes  = 255
)

var (
	ErrInvalidSubject = errors.New("oauth identity has no valid subject")
	fieldNamePattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

type Profile struct {
	Subject string
	Login   string
	Email   string
	Name    string
}

type standardClaims struct {
	Login             string `json:"login"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

// DecodeOAuth2 extracts a provider-specific, top-level identity field from an
// OAuth2 UserInfo response. OIDC subjects must use DecodeOIDCClaims instead.
func DecodeOAuth2(r io.Reader, subjectField string) (Profile, error) {
	if err := ValidateOAuth2SubjectField(subjectField); err != nil {
		return Profile{}, err
	}

	data, err := readLimitedJSON(r)
	if err != nil {
		return Profile{}, err
	}

	var fields map[string]json.RawMessage
	if err = decodeSingleJSON(data, &fields); err != nil {
		return Profile{}, err
	}
	var claims standardClaims
	if err = json.Unmarshal(data, &claims); err != nil {
		return Profile{}, fmt.Errorf("decode userinfo claims: %w", err)
	}

	subject, err := subjectFromJSON(fields[subjectField])
	if err != nil {
		return Profile{}, err
	}
	return profileFromClaims(subject, claims), nil
}

// DecodeOIDCClaims accepts claims only after their ID Token has been verified
// by the caller. It never falls back to a provider-specific OAuth2 ID.
func DecodeOIDCClaims(subject string, rawClaims []byte) (Profile, error) {
	subject, err := ValidateSubject(subject)
	if err != nil {
		return Profile{}, err
	}

	var claims standardClaims
	if err = decodeSingleJSON(rawClaims, &claims); err != nil {
		return Profile{}, err
	}
	return profileFromClaims(subject, claims), nil
}

func ValidateSubject(subject string) (string, error) {
	if subject == "" || subject == "0" || len(subject) > maxSubjectBytes || strings.TrimSpace(subject) != subject {
		return "", ErrInvalidSubject
	}
	return subject, nil
}

func ValidateOAuth2SubjectField(field string) error {
	if !fieldNamePattern.MatchString(field) || field == "sub" {
		return errors.New("invalid OAuth2 subject field")
	}
	return nil
}

func profileFromClaims(subject string, claims standardClaims) Profile {
	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}
	return Profile{
		Subject: subject,
		Login:   claims.Login,
		Email:   claims.Email,
		Name:    name,
	}
}

func subjectFromJSON(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", ErrInvalidSubject
	}

	var stringID string
	if err := json.Unmarshal(raw, &stringID); err == nil {
		return ValidateSubject(stringID)
	}

	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return "", ErrInvalidSubject
	}
	value, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil || value == 0 {
		return "", ErrInvalidSubject
	}
	return strconv.FormatUint(value, 10), nil
}

func readLimitedJSON(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxUserInfoBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read userinfo: %w", err)
	}
	if len(data) > maxUserInfoBytes {
		return nil, errors.New("oauth userinfo response is too large")
	}
	return data, nil
}

func decodeSingleJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode identity claims: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing identity data: %w", err)
	}
	return errors.New("identity data contains multiple JSON values")
}
