// Пакет hmac реализует подпись и проверку вебхуков платформы CBL.
// Подпись считается по сырому телу запроса, окно replay ограничено.
package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DefaultWindow задает допустимое окно расхождения времени подписи.
const DefaultWindow = 5 * time.Minute

// Ошибки пакета возвращаются обернутыми через fmt.Errorf с %w.
var (
	// ErrMalformed сообщает о некорректном заголовке подписи.
	ErrMalformed = errors.New("hmac: malformed header")
	// ErrSignatureMismatch сообщает о несовпадении подписи.
	ErrSignatureMismatch = errors.New("hmac: signature mismatch")
	// ErrExpired сообщает о выходе метки времени за окно replay.
	ErrExpired = errors.New("hmac: timestamp outside replay window")
)

// Signature описывает одну подпись для сборки значения заголовка.
type Signature struct {
	// Timestamp задает время подписи.
	Timestamp time.Time
	// Hex задает hex-представление HMAC-SHA256.
	Hex string
}

// Sign вычисляет hex HMAC-SHA256(secret, "<unix>.<body>").
func Sign(secret []byte, timestamp time.Time, body []byte) string {
	return hex.EncodeToString(signRaw(secret, timestamp.Unix(), body))
}

// BuildHeader собирает заголовок вида t=<unix>,v1=<hex>[,v1=<hex>].
// Все подписи используют метку времени первой из них.
func BuildHeader(signatures ...Signature) string {
	if len(signatures) == 0 {
		return ""
	}
	parts := make([]string, 0, len(signatures)+1)
	parts = append(parts, "t="+strconv.FormatInt(signatures[0].Timestamp.Unix(), 10))
	for _, s := range signatures {
		parts = append(parts, "v1="+s.Hex)
	}
	return strings.Join(parts, ",")
}

// ParseHeader разбирает заголовок на метку времени и список подписей.
// Неизвестные части игнорируются для прямой совместимости.
func ParseHeader(header string) (time.Time, [][]byte, error) {
	var ts int64 = -1
	var sigs [][]byte
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		if !found {
			return time.Time{}, nil, fmt.Errorf("%w: %q", ErrMalformed, part)
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed < 0 {
				return time.Time{}, nil, fmt.Errorf("%w: timestamp %q", ErrMalformed, value)
			}
			ts = parsed
		case "v1":
			raw, err := hex.DecodeString(value)
			if err != nil || len(raw) != sha256.Size {
				return time.Time{}, nil, fmt.Errorf("%w: signature %q", ErrMalformed, value)
			}
			sigs = append(sigs, raw)
		}
	}
	if ts < 0 {
		return time.Time{}, nil, fmt.Errorf("%w: timestamp missing", ErrMalformed)
	}
	if len(sigs) == 0 {
		return time.Time{}, nil, fmt.Errorf("%w: signatures missing", ErrMalformed)
	}
	return time.Unix(ts, 0).UTC(), sigs, nil
}

// Verify проверяет подпись заголовка для одного секрета.
func Verify(secret []byte, header string, body []byte, now time.Time, window time.Duration) error {
	return VerifyAny([][]byte{secret}, header, body, now, window)
}

// VerifyAny проверяет, что одна из подписей заголовка совпадает с одним из секретов.
// Несколько секретов нужны для ротации секрета без простоя.
func VerifyAny(secrets [][]byte, header string, body []byte, now time.Time, window time.Duration) error {
	ts, sigs, err := ParseHeader(header)
	if err != nil {
		return err
	}
	if window <= 0 {
		window = DefaultWindow
	}
	delta := now.Sub(ts)
	if delta < 0 {
		delta = -delta
	}
	if delta > window {
		return fmt.Errorf("%w: delta %s", ErrExpired, delta)
	}
	for _, secret := range secrets {
		if len(secret) == 0 {
			continue
		}
		expected := signRaw(secret, ts.Unix(), body)
		for _, sig := range sigs {
			if hmac.Equal(expected, sig) {
				return nil
			}
		}
	}
	return ErrSignatureMismatch
}

// signRaw вычисляет сырой HMAC-SHA256 по правилу платформы.
func signRaw(secret []byte, unix int64, body []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(unix, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return mac.Sum(nil)
}
