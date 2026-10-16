package webhooksig_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cbl/cbl-kit/webhooksig"
)

func header(name, value string) http.Header {
	h := http.Header{}
	h.Set(name, value)
	return h
}

func TestRawHMACHexWithPrefix(t *testing.T) {
	t.Parallel()
	secret := []byte("btcpay-secret")
	body := []byte(`{"deliveryId":"abc"}`)
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	value := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	strategy := webhooksig.RawHMAC{Header: "BTCPay-Sig", Prefix: "sha256="}
	req := webhooksig.Request{Header: header("Btcpay-Sig", value), Body: body}
	require.NoError(t, strategy.Verify(secret, req))

	require.ErrorIs(t, strategy.Verify([]byte("other"), req), webhooksig.ErrSignatureMismatch)

	wrongBody := webhooksig.Request{Header: req.Header, Body: []byte(`{"deliveryId":"xyz"}`)}
	require.ErrorIs(t, strategy.Verify(secret, wrongBody), webhooksig.ErrSignatureMismatch)

	require.ErrorIs(t, strategy.Verify(secret, webhooksig.Request{Body: body}), webhooksig.ErrSignatureAbsent)

	noPrefix := webhooksig.Request{Header: header("Btcpay-Sig", hex.EncodeToString(mac.Sum(nil))), Body: body}
	require.ErrorIs(t, strategy.Verify(secret, noPrefix), webhooksig.ErrMalformed)

	badHex := webhooksig.Request{Header: header("Btcpay-Sig", "sha256=zzzz"), Body: body}
	require.ErrorIs(t, strategy.Verify(secret, badHex), webhooksig.ErrMalformed)

	longGarbage := webhooksig.Request{Header: header("Btcpay-Sig", "sha256="+strings.Repeat("z", 200)), Body: body}
	require.ErrorIs(t, strategy.Verify(secret, longGarbage), webhooksig.ErrMalformed)

	require.ErrorIs(t, strategy.Verify(nil, req), webhooksig.ErrMalformed)

	require.ErrorIs(t, webhooksig.RawHMAC{}.Verify(secret, req), webhooksig.ErrMalformed)

	badAlg := webhooksig.RawHMAC{Header: "BTCPay-Sig", Algorithm: webhooksig.Algorithm(9)}
	require.ErrorIs(t, badAlg.Verify(secret, req), webhooksig.ErrMalformed)
}

func TestRawHMACSHA512Base64(t *testing.T) {
	t.Parallel()
	secret := []byte("bitpay-token")
	body := []byte(`{"id":"inv-1"}`)
	mac := hmac.New(sha512.New, secret)
	mac.Write(body)
	value := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	strategy := webhooksig.RawHMAC{Header: "X-Signature", Algorithm: webhooksig.SHA512, Encoding: webhooksig.Base64}
	req := webhooksig.Request{Header: header("x-signature", value), Body: body}
	require.NoError(t, strategy.Verify(secret, req))

	badBase64 := webhooksig.Request{Header: header("x-signature", "!!!not-base64!!!"), Body: body}
	require.ErrorIs(t, strategy.Verify(secret, badBase64), webhooksig.ErrMalformed)

	badEnc := webhooksig.RawHMAC{Header: "X-Signature", Encoding: webhooksig.Encoding(9)}
	require.ErrorIs(t, badEnc.Verify(secret, req), webhooksig.ErrMalformed)
}

func TestCanonicalJSONHMAC(t *testing.T) {
	t.Parallel()
	secret := []byte("now-secret")
	body := []byte(`{"price_amount": 10.5, "order_id": "a", "price_currency": "usd"}`)
	canonical, err := webhooksig.CanonicalJSON(body)
	require.NoError(t, err)
	mac := hmac.New(sha512.New, secret)
	mac.Write(canonical)

	strategy := webhooksig.CanonicalJSONHMAC{Header: "x-nowpayments-sig", Algorithm: webhooksig.SHA512}
	req := webhooksig.Request{Header: header("x-nowpayments-sig", hex.EncodeToString(mac.Sum(nil))), Body: body}
	require.NoError(t, strategy.Verify(secret, req))

	rawMac := hmac.New(sha512.New, secret)
	rawMac.Write(body)
	rawReq := webhooksig.Request{Header: header("x-nowpayments-sig", hex.EncodeToString(rawMac.Sum(nil))), Body: body}
	require.ErrorIs(t, strategy.Verify(secret, rawReq), webhooksig.ErrSignatureMismatch)

	broken := webhooksig.Request{Header: req.Header, Body: []byte(`{"a":`)}
	require.ErrorIs(t, strategy.Verify(secret, broken), webhooksig.ErrMalformed)
}

func TestCanonicalJSONHMACBase64(t *testing.T) {
	t.Parallel()
	secret := []byte("now-secret")
	body := []byte(`{"price_amount": 10.5, "order_id": "a"}`)
	canonical, err := webhooksig.CanonicalJSON(body)
	require.NoError(t, err)
	mac := hmac.New(sha512.New, secret)
	mac.Write(canonical)

	strategy := webhooksig.CanonicalJSONHMAC{Header: "X-Sig", Algorithm: webhooksig.SHA512, Encoding: webhooksig.Base64}
	req := webhooksig.Request{Header: header("x-sig", base64.StdEncoding.EncodeToString(mac.Sum(nil))), Body: body}
	require.NoError(t, strategy.Verify(secret, req))
}

func TestTokenEcho(t *testing.T) {
	t.Parallel()
	secret := []byte("echo-token")
	body := []byte(`{"order_id":1,"token":"echo-token"}`)

	strategy := webhooksig.TokenEcho{}
	req := webhooksig.Request{Body: body}
	require.NoError(t, strategy.Verify(secret, req))

	require.ErrorIs(t, strategy.Verify([]byte("nope"), req), webhooksig.ErrEchoMismatch)

	customBody := []byte(`{"payment_token":"echo-token"}`)
	custom := webhooksig.TokenEcho{Field: "payment_token"}
	require.NoError(t, custom.Verify(secret, webhooksig.Request{Body: customBody}))

	missing := webhooksig.Request{Body: []byte(`{"order_id":1}`)}
	require.ErrorIs(t, strategy.Verify(secret, missing), webhooksig.ErrSignatureAbsent)

	notString := webhooksig.Request{Body: []byte(`{"token":1}`)}
	require.ErrorIs(t, strategy.Verify(secret, notString), webhooksig.ErrMalformed)

	broken := webhooksig.Request{Body: []byte(`{`)}
	require.ErrorIs(t, strategy.Verify(secret, broken), webhooksig.ErrMalformed)

	notObject := webhooksig.Request{Body: []byte(`["token"]`)}
	require.ErrorIs(t, strategy.Verify(secret, notObject), webhooksig.ErrMalformed)

	trailing := webhooksig.Request{Body: []byte(`{"token":"echo-token"} garbage`)}
	require.ErrorIs(t, strategy.Verify(secret, trailing), webhooksig.ErrMalformed)

	duplicate := webhooksig.Request{Body: []byte(`{"token":"echo-token","token":"echo-token"}`)}
	require.ErrorIs(t, strategy.Verify(secret, duplicate), webhooksig.ErrMalformed)

	require.ErrorIs(t, strategy.Verify(nil, req), webhooksig.ErrMalformed)
}
